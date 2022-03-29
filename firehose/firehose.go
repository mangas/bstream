package firehose

import (
	"context"
	"errors"
	"fmt"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/forkable"
	"github.com/streamingfast/dstore"
	"go.uber.org/zap"
)

type Firehose struct {
	chainConfig *bstream.ChainConfig

	liveSourceFactory bstream.SourceFactory
	blocksStores      []dstore.Store

	startBlockNum                  int64
	stopBlockNum                   uint64
	irreversibleBlocksIndexStore   dstore.Store
	irreversibleBlocksIndexBundles []uint64

	handler            bstream.Handler
	preprocessFunc     bstream.PreprocessFunc
	blockIndexProvider bstream.BlockIndexProvider

	cursor    *bstream.Cursor
	forkSteps bstream.StepType
	tracker   *bstream.Tracker

	liveHeadTracker           bstream.BlockRefGetter
	logger                    *zap.Logger
	confirmations             uint64
	streamBlocksParallelFiles int
}

// New creates a new Firehose instance configured using the provide options
func New(
	chain *bstream.ChainConfig,
	blocksStores []dstore.Store,
	startBlockNum int64,
	handler bstream.Handler,
	options ...Option) *Firehose {
	f := &Firehose{
		chainConfig:               chain,
		blocksStores:              blocksStores,
		startBlockNum:             startBlockNum,
		logger:                    zlog,
		forkSteps:                 bstream.StepsAll,
		handler:                   handler,
		streamBlocksParallelFiles: 1,
	}

	for _, option := range options {
		option(f)
	}

	return f
}

func (f *Firehose) Run(ctx context.Context) error {
	source, err := f.createSource(ctx)
	if err != nil {
		return err
	}

	go func() {
		select {
		case <-ctx.Done():
			source.Shutdown(ctx.Err())
		}
	}()

	source.Run()
	if err := source.Err(); err != nil {
		f.logger.Debug("source shutting down", zap.Error(err))
		return err
	}
	return nil
}

func (f *Firehose) createSource(ctx context.Context) (bstream.Source, error) {
	f.logger.Debug("setting up firehose source")

	absoluteStartBlockNum, err := resolveNegativeStartBlockNum(ctx, f.startBlockNum, f.tracker)
	if err != nil {
		return nil, err
	}
	if absoluteStartBlockNum < f.chainConfig.FirstStreamableBlock {
		absoluteStartBlockNum = f.chainConfig.FirstStreamableBlock
	}
	if f.stopBlockNum > 0 && absoluteStartBlockNum > f.stopBlockNum {
		return nil, NewErrInvalidArg("start block %d is after stop block %d", absoluteStartBlockNum, f.stopBlockNum)
	}

	hasCursor := !f.cursor.IsEmpty()

	if f.irreversibleBlocksIndexStore != nil {
		var cursorBlock bstream.BlockRef
		var forkedCursor bool
		irreversibleStartBlockNum := absoluteStartBlockNum

		if hasCursor {
			cursorBlock = f.cursor.Block
			if f.cursor.Step != bstream.StepNew && f.cursor.Step != bstream.StepIrreversible {
				forkedCursor = true
			}
			irreversibleStartBlockNum = f.cursor.LIB.Num()
		}

		if !forkedCursor {
			if irrIndex := bstream.NewBlockIndexesManager(ctx, f.irreversibleBlocksIndexStore, f.irreversibleBlocksIndexBundles, irreversibleStartBlockNum, f.stopBlockNum, cursorBlock, f.blockIndexProvider); irrIndex != nil {
				return bstream.NewIndexedFileSource(
					f.chainConfig,
					f.wrappedHandler(),
					f.preprocessFunc,
					irrIndex,
					f.blocksStores,
					f.joiningSourceFactory(),
					f.forkableHandlerWrapper(nil, false, 0),
					f.logger,
					f.forkSteps,
					f.cursor,
				), nil
			}
		}
	}

	// joiningSource -> forkable -> wrappedHandler
	h := f.wrappedHandler()
	if hasCursor {
		forkableHandlerWrapper := f.forkableHandlerWrapper(f.cursor, true, absoluteStartBlockNum) // you don't want the cursor's block to be the lower limit
		forkableHandler := forkableHandlerWrapper(h, f.cursor.LIB)
		jsf := f.joiningSourceFactoryFromCursor(f.cursor)

		return jsf(f.cursor.Block.Num(), forkableHandler), nil
	}

	if f.tracker != nil {
		irreversibleStartBlockNum, previousIrreversibleID, err := f.tracker.ResolveStartBlock(ctx, absoluteStartBlockNum)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve start block: %w", err)
		}
		var irrRef bstream.BlockRef
		if previousIrreversibleID != "" {
			irrRef = bstream.NewBlockRef(previousIrreversibleID, irreversibleStartBlockNum)
		}

		forkableHandlerWrapper := f.forkableHandlerWrapper(nil, true, absoluteStartBlockNum)
		forkableHandler := forkableHandlerWrapper(h, irrRef)
		jsf := f.joiningSourceFactoryFromResolvedBlock(irreversibleStartBlockNum, previousIrreversibleID)
		return jsf(absoluteStartBlockNum, forkableHandler), nil
	}

	// no cursor, no tracker, probably just block files on disk
	forkableHandlerWrapper := f.forkableHandlerWrapper(nil, false, absoluteStartBlockNum)
	forkableHandler := forkableHandlerWrapper(h, nil)
	jsf := f.joiningSourceFactory()
	return jsf(absoluteStartBlockNum, forkableHandler), nil

}

func resolveNegativeStartBlockNum(ctx context.Context, startBlockNum int64, tracker *bstream.Tracker) (uint64, error) {
	if startBlockNum < 0 {
		absoluteValue, err := tracker.GetRelativeBlock(ctx, startBlockNum, bstream.BlockStreamHeadTarget)
		if err != nil {
			if errors.Is(err, bstream.ErrGetterUndefined) {
				return 0, NewErrInvalidArg("requested negative start block number (%d), but this instance has no HEAD tracker", startBlockNum)
			}
			return 0, fmt.Errorf("getting relative block: %w", err)
		}
		return absoluteValue, nil
	}
	return uint64(startBlockNum), nil
}

// adds stopBlock to the handler
func (f *Firehose) wrappedHandler() bstream.Handler {

	h := f.handler

	if f.stopBlockNum > 0 {
		h = bstream.HandlerFunc(func(block *bstream.Block, obj interface{}) error {
			if block.Number > f.stopBlockNum {
				return ErrStopBlockReached
			}
			if err := f.handler.ProcessBlock(block, obj); err != nil {
				return err
			}

			if block.Number == f.stopBlockNum {
				return ErrStopBlockReached
			}
			return nil
		})
	}

	return h

}

func (f *Firehose) forkableHandlerWrapper(cursor *bstream.Cursor, libInclusive bool, startBlockNum uint64) func(h bstream.Handler, lib bstream.BlockRef) bstream.Handler {
	return func(h bstream.Handler, lib bstream.BlockRef) bstream.Handler {

		forkableOptions := []forkable.Option{
			forkable.WithLogger(f.logger),
			forkable.WithFilters(f.forkSteps),
		}

		if f.confirmations != 0 {
			f.logger.Info("confirmations threshold configured, added relative LIB num getter to pipeline", zap.Uint64("confirmations", f.confirmations))
			forkableOptions = append(forkableOptions,
				forkable.WithCustomLIBNumGetter(forkable.RelativeLIBNumGetter(f.chainConfig.FirstStreamableBlock, f.confirmations)))
		}

		if !cursor.IsEmpty() {
			// does all the heavy lifting (setting the lib and start block, etc.)
			forkableOptions = append(forkableOptions, forkable.FromCursor(f.cursor))
		} else {
			if lib != nil {
				if libInclusive {
					f.logger.Debug("configuring inclusive LIB on forkable handler", zap.Stringer("lib", lib))
					forkableOptions = append(forkableOptions, forkable.WithInclusiveLIB(lib))
				} else {
					f.logger.Debug("configuring exclusive LIB on forkable handler", zap.Stringer("lib", lib))
					forkableOptions = append(forkableOptions, forkable.WithExclusiveLIB(lib))
				}
			}
		}

		return forkable.New(f.chainConfig, bstream.NewMinimalBlockNumFilter(startBlockNum, h), forkableOptions...)
	}
}

func (f *Firehose) joiningSourceFactoryFromResolvedBlock(fileStartBlock uint64, previousIrreversibleID string) bstream.SourceFromNumFactory {
	return func(startBlockNum uint64, h bstream.Handler) bstream.Source {

		joiningSourceOptions := []bstream.JoiningSourceOption{
			bstream.JoiningSourceLogger(f.logger),
			bstream.JoiningSourceStartLiveImmediately(false),
		}

		if f.liveHeadTracker != nil {
			joiningSourceOptions = append(joiningSourceOptions, bstream.JoiningSourceLiveTracker(120, f.liveHeadTracker))
		}

		f.logger.Info("firehose pipeline bootstrapping from tracker",
			zap.Uint64("requested_start_block", startBlockNum),
			zap.Uint64("file_start_block", fileStartBlock),
			zap.String("previous_irr_id", previousIrreversibleID),
		)

		if previousIrreversibleID != "" {
			joiningSourceOptions = append(joiningSourceOptions, bstream.JoiningSourceTargetBlockID(previousIrreversibleID))
		}

		return bstream.NewJoiningSource(f.chainConfig, f.fileSourceFactory(fileStartBlock), f.liveSourceFactory, h, joiningSourceOptions...)

	}
}

func (f *Firehose) joiningSourceFactoryFromCursor(cursor *bstream.Cursor) bstream.SourceFromNumFactory {
	return func(startBlockNum uint64, h bstream.Handler) bstream.Source {

		joiningSourceOptions := []bstream.JoiningSourceOption{
			bstream.JoiningSourceLogger(f.logger),
			bstream.JoiningSourceStartLiveImmediately(false),
		}

		if f.liveHeadTracker != nil {
			joiningSourceOptions = append(joiningSourceOptions, bstream.JoiningSourceLiveTracker(120, f.liveHeadTracker))
		}

		fileStartBlock := cursor.LIB.Num() // we don't use startBlockNum, the forkable will wait for the cursor before it forwards blocks
		firstStreamableBlock := f.chainConfig.FirstStreamableBlock
		if fileStartBlock < firstStreamableBlock {
			f.logger.Info("adjusting requested file_start_block to protocol_first_streamable_block",
				zap.Uint64("file_start_block", fileStartBlock),
				zap.Uint64("protocol_first_streamable_block", firstStreamableBlock),
			)
			fileStartBlock = firstStreamableBlock
		}
		joiningSourceOptions = append(joiningSourceOptions, bstream.JoiningSourceTargetBlockID(cursor.LIB.ID()))

		f.logger.Info("firehose pipeline bootstrapping from cursor",
			zap.Uint64("file_start_block", fileStartBlock),
			zap.Stringer("cursor_lib", cursor.LIB),
		)
		return bstream.NewJoiningSource(f.chainConfig, f.fileSourceFactory(fileStartBlock), f.liveSourceFactory, h, joiningSourceOptions...)
	}
}

func (f *Firehose) joiningSourceFactory() bstream.SourceFromNumFactory {
	return func(startBlockNum uint64, h bstream.Handler) bstream.Source {
		joiningSourceOptions := []bstream.JoiningSourceOption{
			bstream.JoiningSourceLogger(f.logger),
			bstream.JoiningSourceStartLiveImmediately(false),
		}
		f.logger.Info("firehose pipeline bootstrapping",
			zap.Uint64("start_block", startBlockNum),
		)
		return bstream.NewJoiningSource(f.chainConfig, f.fileSourceFactory(startBlockNum), f.liveSourceFactory, h, joiningSourceOptions...)
	}
}

func (f *Firehose) fileSourceFactory(startBlockNum uint64) bstream.SourceFactory {
	return func(h bstream.Handler) bstream.Source {
		var fileSourceOptions []bstream.FileSourceOption
		if len(f.blocksStores) > 1 {
			fileSourceOptions = append(fileSourceOptions, bstream.FileSourceWithSecondaryBlocksStores(f.blocksStores[1:]))
		}
		fileSourceOptions = append(fileSourceOptions, bstream.FileSourceWithConcurrentPreprocess(f.streamBlocksParallelFiles))

		fs := bstream.NewFileSource(
			f.chainConfig,
			f.blocksStores[0],
			startBlockNum,
			f.streamBlocksParallelFiles,
			f.preprocessFunc,
			h,
			fileSourceOptions...,
		)
		return fs
	}
}
