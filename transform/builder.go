package transform

import (
	"fmt"
	"github.com/streamingfast/bstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func BuildFromTransforms(anyTransforms []*anypb.Any) (bstream.PreprocessFunc, error) {
	transforms := []Transform{}
	for _, transform := range anyTransforms {
		t, err := New(transform)
		if err != nil {
			return nil, fmt.Errorf("unable to instantiate transform: %w", err)
		}
		transforms = append(transforms, t)
	}

	var in Input
	return func(blk *bstream.Block) (interface{}, error) {
		in = NewNilObj()
		var out proto.Message
		var err error
		for idx, transform := range transforms {
			if out, err = transform.Transform(blk, in); err != nil {
				return nil, fmt.Errorf("transform %d failed: %w", idx, err)
			}
			in = &InputObj{
				_type: string(proto.MessageName(out)),
				obj:   out,
			}
		}
		return out, nil
	}, nil
}
