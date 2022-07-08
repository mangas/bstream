package hub

import (
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/forkable"
	"github.com/streamingfast/shutter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForkableHub_Bootstrap(t *testing.T) {
	tests := []struct {
		name                       string
		liveBlocks                 []*bstream.Block
		oneBlocksPasses            [][]*bstream.Block
		bufferSize                 int
		expectStartNum             uint64
		expectReady                bool
		expectReadyAfter           bool
		expectHeadRef              bstream.BlockRef
		expectBlocksInCurrentChain []uint64
	}{
		{
			name: "vanilla",
			liveBlocks: []*bstream.Block{
				bstream.TestBlockWithLIBNum("00000009", "00000008", 3),
				bstream.TestBlockWithLIBNum("0000000a", "00000009", 4),
			},
			oneBlocksPasses: [][]*bstream.Block{{
				bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
				bstream.TestBlockWithLIBNum("00000004", "00000003", 2),
				bstream.TestBlockWithLIBNum("00000005", "00000004", 2),
				bstream.TestBlockWithLIBNum("00000008", "00000005", 3),
			}},
			bufferSize:                 0,
			expectStartNum:             0,
			expectReady:                true,
			expectReadyAfter:           true,
			expectHeadRef:              bstream.NewBlockRefFromID("00000009"),
			expectBlocksInCurrentChain: []uint64{4, 5, 8, 9},
		},
		{
			name: "LIB met on second live block",
			liveBlocks: []*bstream.Block{
				bstream.TestBlockWithLIBNum("00000009", "00000008", 3),
				bstream.TestBlockWithLIBNum("0000000a", "00000009", 5),
			},
			oneBlocksPasses: [][]*bstream.Block{
				{
					bstream.TestBlockWithLIBNum("00000005", "00000004", 2),
					bstream.TestBlockWithLIBNum("00000008", "00000005", 3),
				},
				{
					bstream.TestBlockWithLIBNum("00000005", "00000004", 2),
					bstream.TestBlockWithLIBNum("00000008", "00000005", 3),
				},
			},
			bufferSize:                 0,
			expectStartNum:             0,
			expectReady:                false,
			expectReadyAfter:           true,
			expectHeadRef:              bstream.NewBlockRefFromID("0000000a"),
			expectBlocksInCurrentChain: []uint64{5, 8, 9, 10},
		},
		{
			name: "cannot join one-block-files",
			liveBlocks: []*bstream.Block{
				bstream.TestBlockWithLIBNum("00000009", "00000008", 3),
				bstream.TestBlockWithLIBNum("0000000a", "00000009", 4),
			},
			oneBlocksPasses: [][]*bstream.Block{
				{
					bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
					bstream.TestBlockWithLIBNum("00000004", "00000003", 3),
					bstream.TestBlockWithLIBNum("00000005", "00000004", 3),
				},
				{
					bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
					bstream.TestBlockWithLIBNum("00000004", "00000003", 3),
					bstream.TestBlockWithLIBNum("00000005", "00000004", 3),
				},
			},
			bufferSize:       0,
			expectStartNum:   0,
			expectReady:      false,
			expectReadyAfter: false,
		},

		{
			name: "one-block-file joined eventually",
			liveBlocks: []*bstream.Block{
				bstream.TestBlockWithLIBNum("00000008", "00000007", 3),
				bstream.TestBlockWithLIBNum("00000009", "00000008", 3),
			},
			oneBlocksPasses: [][]*bstream.Block{
				{
					bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
					bstream.TestBlockWithLIBNum("00000004", "00000003", 3),
					bstream.TestBlockWithLIBNum("00000006", "00000004", 3),
				},
				{
					bstream.TestBlockWithLIBNum("00000003", "00000002", 3),
					bstream.TestBlockWithLIBNum("00000004", "00000003", 3),
					bstream.TestBlockWithLIBNum("00000006", "00000004", 3),
					bstream.TestBlockWithLIBNum("00000007", "00000006", 3),
				},
			},
			bufferSize:                 2,
			expectStartNum:             0,
			expectReady:                false,
			expectReadyAfter:           true,
			expectHeadRef:              bstream.NewBlockRefFromID("00000009"),
			expectBlocksInCurrentChain: []uint64{3, 4, 6, 7, 8, 9},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lsf := bstream.NewTestSourceFactory()
			obsf := bstream.NewTestSourceFactory()
			fh := NewForkableHub(lsf.NewSource, obsf.NewSourceFromNum, test.bufferSize)

			go fh.Run()

			ls := <-lsf.Created

			// sending oneblockfiles on demand
			go func() {
				for _, oneBlocks := range test.oneBlocksPasses {
					obs := <-obsf.Created
					assert.Equal(t, test.expectStartNum, obs.StartBlockNum)
					for _, blk := range oneBlocks {
						require.NoError(t, obs.Push(blk, nil))
					}
					obs.Shutdown(io.EOF)
				}
			}()

			if err := ls.Push(test.liveBlocks[0], nil); err != nil {
				ls.Shutdown(err)
			}

			assert.Equal(t, test.expectReady, fh.Ready)

			for _, blk := range test.liveBlocks[1:] {
				require.NoError(t, ls.Push(blk, nil))
			}
			assert.Equal(t, test.expectReadyAfter, fh.Ready)

			if test.expectHeadRef != nil || test.expectBlocksInCurrentChain != nil {
				var seenBlockNums []uint64
				chain, _ := fh.forkdb.CompleteSegment(test.expectHeadRef)
				for _, blk := range chain {
					seenBlockNums = append(seenBlockNums, blk.BlockNum)
				}
				assert.Equal(t, test.expectBlocksInCurrentChain, seenBlockNums)
			}

			assert.False(t, fh.IsTerminating())

		})
	}
}

func TestForkableHub_SourceFromFinalBlock(t *testing.T) {

	type expectedBlock struct {
		block        *bstream.Block
		step         bstream.StepType
		cursorLibNum uint64
	}

	tests := []struct {
		name         string
		forkdbBlocks []*bstream.Block
		requestBlock bstream.BlockRef
		expectBlocks []expectedBlock
	}{
		{
			name: "vanilla",
			forkdbBlocks: []*bstream.Block{
				bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
				bstream.TestBlockWithLIBNum("00000004", "00000003", 2),
				bstream.TestBlockWithLIBNum("00000005", "00000004", 2),
				bstream.TestBlockWithLIBNum("00000008", "00000005", 3),
				bstream.TestBlockWithLIBNum("00000009", "00000008", 3),
				bstream.TestBlockWithLIBNum("0000000a", "00000009", 4),
			},
			requestBlock: bstream.NewBlockRefFromID("00000005"),
			expectBlocks: []expectedBlock{
				{
					bstream.TestBlockWithLIBNum("00000005", "00000004", 2),
					bstream.StepNew,
					4,
				},

				{
					bstream.TestBlockWithLIBNum("00000008", "00000005", 3),
					bstream.StepNew,
					4,
				},
				{
					bstream.TestBlockWithLIBNum("00000009", "00000008", 3),
					bstream.StepNew,
					4,
				},

				{
					bstream.TestBlockWithLIBNum("0000000a", "00000009", 4),
					bstream.StepNew,
					4,
				},
			},
		},
		{
			name: "step_irreversible",
			forkdbBlocks: []*bstream.Block{
				bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
				bstream.TestBlockWithLIBNum("00000004", "00000003", 2),
				bstream.TestBlockWithLIBNum("00000005", "00000004", 2),
				bstream.TestBlockWithLIBNum("00000008", "00000005", 4),
				bstream.TestBlockWithLIBNum("00000009", "00000008", 5),
				bstream.TestBlockWithLIBNum("0000000a", "00000009", 8),
			},
			requestBlock: bstream.NewBlockRefFromID("00000003"),
			expectBlocks: []expectedBlock{
				{
					bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
					bstream.StepNewIrreversible,
					3,
				},
				{
					bstream.TestBlockWithLIBNum("00000004", "00000003", 2),
					bstream.StepNewIrreversible,
					4,
				},
				{
					bstream.TestBlockWithLIBNum("00000005", "00000004", 2),
					bstream.StepNewIrreversible,
					5, // LIB set to itself
				},

				{
					bstream.TestBlockWithLIBNum("00000008", "00000005", 4),
					bstream.StepNewIrreversible,
					8, // real current hub LIB
				},
				{
					bstream.TestBlockWithLIBNum("00000009", "00000008", 5),
					bstream.StepNew,
					8,
				},

				{
					bstream.TestBlockWithLIBNum("0000000a", "00000009", 8),
					bstream.StepNew,
					8,
				},
			},
		},

		{
			name: "no source",
			forkdbBlocks: []*bstream.Block{
				bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
				bstream.TestBlockWithLIBNum("00000004", "00000003", 3),
			},
			requestBlock: bstream.NewBlockRefFromID("00000005"),
		},
		{
			name: "no source cause wrong block",
			forkdbBlocks: []*bstream.Block{
				bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
				bstream.TestBlockWithLIBNum("00000004", "00000003", 3),
			},
			requestBlock: bstream.NewBlockRef("00000033", 3),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			fh := &ForkableHub{
				Shutter: shutter.New(),
				forkdb:  forkable.NewForkDB(),
			}
			fh.forkable = forkable.New(fh,
				forkable.WithForkDB(fh.forkdb),
				forkable.HoldBlocksUntilLIB(),
				forkable.WithKeptFinalBlocks(100),
			)
			fh.Ready = true

			for _, blk := range test.forkdbBlocks {
				fh.forkable.ProcessBlock(blk, nil)
			}

			var seenBlocks []expectedBlock
			handler := bstream.HandlerFunc(func(blk *bstream.Block, obj interface{}) error {
				seenBlocks = append(seenBlocks, expectedBlock{blk, obj.(*forkable.ForkableObject).Step(), obj.(*forkable.ForkableObject).Cursor().LIB.Num()})
				if len(seenBlocks) == len(test.expectBlocks) {
					return fmt.Errorf("done")
				}
				return nil
			})
			source := fh.SourceFromFinalBlock(handler, test.requestBlock)
			if test.expectBlocks == nil {
				assert.Nil(t, source)
				return
			}
			go source.Run()
			<-source.Terminating()
			assert.Equal(t, test.expectBlocks, seenBlocks)
		})
	}
}

func TestForkableHub_SourceFromCursor(t *testing.T) {

	type expectedBlock struct {
		block        *bstream.Block
		step         bstream.StepType
		cursorLibNum uint64
	}

	tests := []struct {
		name          string
		forkdbBlocks  []*bstream.Block
		requestCursor *bstream.Cursor
		expectBlocks  []expectedBlock
	}{
		{
			name: "irr and new",
			forkdbBlocks: []*bstream.Block{
				bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
				bstream.TestBlockWithLIBNum("00000004", "00000003", 2),
				bstream.TestBlockWithLIBNum("00000005", "00000004", 3),
				bstream.TestBlockWithLIBNum("00000008", "00000005", 3),
				bstream.TestBlockWithLIBNum("00000009", "00000008", 3),
				bstream.TestBlockWithLIBNum("0000000a", "00000009", 4),
			},
			requestCursor: &bstream.Cursor{
				Step:      bstream.StepNew,
				Block:     bstream.NewBlockRefFromID("00000005"),
				HeadBlock: bstream.NewBlockRefFromID("00000008"),
				LIB:       bstream.NewBlockRefFromID("00000003"),
			},
			expectBlocks: []expectedBlock{
				{
					bstream.TestBlockWithLIBNum("00000004", "00000003", 2),
					bstream.StepIrreversible,
					4,
				},
				{
					bstream.TestBlockWithLIBNum("00000008", "00000005", 3),
					bstream.StepNew,
					4,
				},
				{
					bstream.TestBlockWithLIBNum("00000009", "00000008", 3),
					bstream.StepNew,
					4,
				},
				{
					bstream.TestBlockWithLIBNum("0000000a", "00000009", 4),
					bstream.StepNew,
					4,
				},
			},
		},
		{
			name: "cursor head in future",
			forkdbBlocks: []*bstream.Block{
				bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
				bstream.TestBlockWithLIBNum("00000004", "00000003", 3),
			},
			requestCursor: &bstream.Cursor{
				Step:      bstream.StepNew,
				Block:     bstream.NewBlockRefFromID("00000005"),
				HeadBlock: bstream.NewBlockRefFromID("00000005"),
				LIB:       bstream.NewBlockRefFromID("00000003"),
			},
		},
		{
			name: "cursor block on head",
			forkdbBlocks: []*bstream.Block{
				bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
				bstream.TestBlockWithLIBNum("00000004", "00000003", 3),
			},
			requestCursor: &bstream.Cursor{
				Step:      bstream.StepNew,
				Block:     bstream.NewBlockRefFromID("00000004"),
				HeadBlock: bstream.NewBlockRefFromID("00000004"),
				LIB:       bstream.NewBlockRefFromID("00000003"),
			},
			expectBlocks: []expectedBlock{},
		},
		{
			name: "cursor block on head, lib moved",
			forkdbBlocks: []*bstream.Block{
				bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
				bstream.TestBlockWithLIBNum("00000004", "00000003", 3),
				bstream.TestBlockWithLIBNum("00000005", "00000004", 4),
			},
			requestCursor: &bstream.Cursor{
				Step:      bstream.StepNew,
				Block:     bstream.NewBlockRefFromID("00000005"),
				HeadBlock: bstream.NewBlockRefFromID("00000005"),
				LIB:       bstream.NewBlockRefFromID("00000003"), // hub has LIB at 4
			},
			expectBlocks: []expectedBlock{
				{
					bstream.TestBlockWithLIBNum("00000004", "00000003", 3),
					bstream.StepIrreversible,
					4,
				},
			},
		},
		{
			name: "cursor on forked block",
			forkdbBlocks: []*bstream.Block{
				bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
				bstream.TestBlockWithLIBNum("00000004", "00000003", 3),
				bstream.TestBlockFromJSON(fmt.Sprintf(`{"id":"00000005b","prev":"00000004","number":5,"libnum":3}`)),
				bstream.TestBlockWithLIBNum("00000005", "00000004", 3),
				bstream.TestBlockWithLIBNum("00000006", "00000005", 3),
			},
			requestCursor: &bstream.Cursor{
				Step:      bstream.StepNew,
				Block:     bstream.NewBlockRef("00000005b", 5),
				HeadBlock: bstream.NewBlockRef("00000005b", 5),
				LIB:       bstream.NewBlockRefFromID("00000003"),
			},
			expectBlocks: []expectedBlock{
				{
					bstream.TestBlockFromJSON(fmt.Sprintf(`{"id":"00000005b","prev":"00000004","number":5,"libnum":3}`)),
					bstream.StepUndo,
					3,
				},
				{
					bstream.TestBlockWithLIBNum("00000005", "00000004", 3),
					bstream.StepNew,
					3,
				},
				{
					bstream.TestBlockWithLIBNum("00000006", "00000005", 3),
					bstream.StepNew,
					3,
				},
			},
		},
		{
			name: "cursor on undo step",
			forkdbBlocks: []*bstream.Block{
				bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
				bstream.TestBlockWithLIBNum("00000004", "00000003", 3),
				bstream.TestBlockFromJSON(fmt.Sprintf(`{"id":"00000005b","prev":"00000004","number":5,"libnum":3}`)),
				bstream.TestBlockWithLIBNum("00000005", "00000004", 3),
				bstream.TestBlockWithLIBNum("00000006", "00000005", 3),
			},
			requestCursor: &bstream.Cursor{
				Step:      bstream.StepUndo,
				Block:     bstream.NewBlockRef("00000005b", 5),
				HeadBlock: bstream.NewBlockRef("00000005b", 5),
				LIB:       bstream.NewBlockRefFromID("00000003"),
			},
			expectBlocks: []expectedBlock{
				{
					bstream.TestBlockWithLIBNum("00000005", "00000004", 3),
					bstream.StepNew,
					3,
				},
				{
					bstream.TestBlockWithLIBNum("00000006", "00000005", 3),
					bstream.StepNew,
					3,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			fh := &ForkableHub{
				Shutter: shutter.New(),
				forkdb:  forkable.NewForkDB(),
			}
			fh.forkable = forkable.New(fh,
				forkable.WithForkDB(fh.forkdb),
				forkable.HoldBlocksUntilLIB(),
				forkable.WithKeptFinalBlocks(100),
			)
			fh.Ready = true

			for _, blk := range test.forkdbBlocks {
				fh.forkable.ProcessBlock(blk, nil)
			}

			var seenBlocks []expectedBlock
			handler := bstream.HandlerFunc(func(blk *bstream.Block, obj interface{}) error {
				seenBlocks = append(seenBlocks, expectedBlock{blk, obj.(*forkable.ForkableObject).Step(), obj.(*forkable.ForkableObject).Cursor().LIB.Num()})
				if len(seenBlocks) == len(test.expectBlocks) {
					return fmt.Errorf("done")
				}
				return nil
			})
			source := fh.SourceFromCursor(handler, test.requestCursor)
			if test.expectBlocks == nil {
				assert.Nil(t, source)
				return
			}

			require.NotNil(t, source)
			fmt.Println("hey got source", source)

			if len(test.expectBlocks) == 0 {
				return // we get an empty source for now, until live blocks come in
			}
			go source.Run()
			select {
			case <-source.Terminating():
				assert.Equal(t, test.expectBlocks, seenBlocks)
			case <-time.After(time.Second):
				t.Errorf("timeout waiting for blocks")
			}
		})
	}
}