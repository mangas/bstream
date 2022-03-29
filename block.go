package bstream

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	pbany "github.com/golang/protobuf/ptypes/any"
	pbbstream "github.com/streamingfast/pbgo/sf/bstream/v1"
)

var GetMemoizeMaxAge time.Duration

// Block reprensents a block abstraction across all dfuse systems
// and for now is wide enough to accomodate a varieties of implementation. It's
// the actual stucture that flows all around `bstream`.
type Block struct {
	chainConfig *ChainConfig

	Id         string
	Number     uint64
	PreviousId string
	Timestamp  time.Time
	LibNum     uint64

	PayloadKind    pbbstream.Protocol
	PayloadVersion int32

	Payload      BlockPayload
	cloned       bool
	memoized     interface{}
	memoizedLock sync.Mutex
}

func NewBlockFromBytes(chain *ChainConfig, bytes []byte) (*Block, error) {
	block := new(pbbstream.Block)
	err := proto.Unmarshal(bytes, block)
	if err != nil {
		return nil, fmt.Errorf("unable to read block from bytes: %w", err)
	}

	return NewBlockFromProto(chain, block)
}

func NewBlockFromProto(chain *ChainConfig, b *pbbstream.Block) (*Block, error) {
	blockTime, err := ptypes.Timestamp(b.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("unable to turn google proto Timestamp %q into time.Time: %w", b.Timestamp.String(), err)
	}

	block := &Block{
		chainConfig: chain,

		Id:             b.Id,
		Number:         b.Number,
		PreviousId:     b.PreviousId,
		Timestamp:      blockTime,
		LibNum:         b.LibNum,
		PayloadKind:    b.PayloadKind,
		PayloadVersion: b.PayloadVersion,
	}

	return chain.BlockPayloadSetter(chain, block, b.PayloadBuffer)
}

func MustNewBlockFromProto(chain *ChainConfig, b *pbbstream.Block) *Block {
	block, err := NewBlockFromProto(chain, b)
	if err != nil {
		panic(err)
	}
	return block
}
func (b *Block) IsCloned() bool {
	return b.cloned
}

func (b *Block) Clone() *Block {
	return &Block{
		chainConfig:    b.chainConfig,
		Id:             b.Id,
		Number:         b.Number,
		PreviousId:     b.PreviousId,
		Timestamp:      b.Timestamp,
		LibNum:         b.LibNum,
		PayloadKind:    b.PayloadKind,
		PayloadVersion: b.PayloadVersion,
		Payload:        b.Payload,
		cloned:         true,
	}
}

func (b *Block) ToAny(decoded bool, interceptor func(blk interface{}) interface{}) (*pbany.Any, error) {
	if decoded {
		blk := b.ToProtocol()
		if interceptor != nil {
			blk = interceptor(blk)
		}

		proto, ok := blk.(proto.Message)
		if !ok {
			return nil, fmt.Errorf("block interface is not of expected type proto.Message, got %T", blk)
		}

		return ptypes.MarshalAny(proto)
	}

	blk, err := b.ToProto()
	if err != nil {
		return nil, fmt.Errorf("to proto: %w", err)
	}

	return ptypes.MarshalAny(blk)
}

func (b *Block) ToProto() (*pbbstream.Block, error) {
	blockTime, err := ptypes.TimestampProto(b.Time())
	if err != nil {
		return nil, fmt.Errorf("unable to transfrom time value %v to proto time: %w", b.Time(), err)
	}

	payload, err := b.Payload.Get()
	if err != nil {
		return nil, fmt.Errorf("retrieving payload for block: %d %s: %w", b.Num(), b.ID(), err)
	}

	return &pbbstream.Block{
		Id:             b.Id,
		Number:         b.Number,
		PreviousId:     b.PreviousId,
		Timestamp:      blockTime,
		LibNum:         b.LibNum,
		PayloadKind:    b.PayloadKind,
		PayloadVersion: b.PayloadVersion,
		PayloadBuffer:  payload,
	}, nil
}

func (b *Block) ID() string {
	if b == nil {
		return ""
	}

	return b.Id
}

func (b *Block) Num() uint64 {
	if b == nil {
		return 0
	}

	return b.Number
}

func (b *Block) PreviousID() string {
	if b == nil {
		return ""
	}

	return b.PreviousId
}

func (b *Block) Time() time.Time {
	if b == nil {
		return time.Time{}
	}

	return b.Timestamp
}

func (b *Block) LIBNum() uint64 {
	if b == nil {
		return 0
	}

	return b.LibNum
}

func (b *Block) Kind() pbbstream.Protocol {
	if b == nil {
		return pbbstream.Protocol_UNKNOWN
	}

	return b.PayloadKind
}

func (b *Block) Version() int32 {
	if b == nil {
		return -1
	}

	return b.PayloadVersion
}

func (b *Block) AsRef() BlockRef {
	if b == nil {
		return BlockRefEmpty
	}

	return NewBlockRef(b.Id, b.Number)
}

func (b *Block) PreviousRef() BlockRef {
	if b == nil || b.Number == 0 || b.PreviousId == "" {
		return BlockRefEmpty
	}

	return NewBlockRef(b.PreviousId, b.Number-1)
}

//func (b *Block) Payload() []byte {
//	if b == nil {
//		return nil
//	}
//
//	// Happens when ToNative has been called once
//	if b.PayloadBuffer == nil && b.memoized != nil {
//		payload, err := proto.Marshal(b.memoized.(proto.Message))
//		if err != nil {
//			panic(fmt.Errorf("unable to re-encode memoized value to payload: %w", err))
//		}
//
//		return payload
//	}
//
//	return b.PayloadBuffer
//}

// Deprecated: ToNative is deprecated, it is replaced by ToProtocol significantly more intuitive naming.
func (b *Block) ToNative() interface{} {
	return b.ToProtocol()
}

func (b *Block) ToProtocol() interface{} {
	if b == nil {
		return nil
	}

	b.memoizedLock.Lock()
	defer b.memoizedLock.Unlock()

	if b.memoized != nil {
		return b.memoized
	}

	obj, err := b.chainConfig.BlockDecoder(b)
	if err != nil {
		panic(fmt.Errorf("unable to decode block kind %s version %d : %w", b.PayloadKind, b.PayloadVersion, err))
	}

	if b.cloned {
		b.memoized = obj
		b.Payload = nil
		return obj
	}
	age := time.Since(b.Time())
	if age < GetMemoizeMaxAge {
		b.memoized = obj
		go func(block *Block) {
			<-time.After(GetMemoizeMaxAge - age)
			block.memoizedLock.Lock()
			b.memoized = nil
			block.memoizedLock.Unlock()
		}(b)
	}

	return obj
}

func (b *Block) String() string {
	return blockRefAsAstring(b)
}
