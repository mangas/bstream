// Copyright 2019 dfuse Platform Inc.  //
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package forkable

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/streamingfast/bstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForkable_ProcessBlockWithCursor(t *testing.T) {
	cases := []struct {
		name           string
		cursor         *bstream.Cursor
		filterSteps    bstream.StepType
		processBlocks  []*bstream.Block
		expectedResult []*ForkableObject
	}{
		{
			// Step:New Block:4a Head:4a LIB: 2a
			name: "cursor step:new simple",
			cursor: &bstream.Cursor{
				Step:      bstream.StepNew,
				LIB:       bTestBlock("00000002a", "00000001a"), // just a Ref, don't fuck matt
				HeadBlock: bTestBlock("00000004a", "00000003a"),
				Block:     bTestBlock("00000004a", "00000003a"),
			},
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"),
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000004a", "00000003a"),
				bTestBlock("00000005a", "00000004a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000005a",
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000005a"),
					lastLIBSent: tinyBlk("00000002a"),
				},
			},
		},
		{

			// First Streamable Block

			name: "cursor step:new first streamable",
			cursor: &bstream.Cursor{
				Step:      bstream.StepNew,
				LIB:       bTestBlock("00000001a", "00000000a"),
				HeadBlock: bTestBlock("00000001a", "00000000a"),
				Block:     bTestBlock("00000001a", "00000000a"),
			},
			processBlocks: []*bstream.Block{
				bTestBlock("00000001a", "00000000a"),
				bTestBlock("00000002a", "00000001a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepIrreversible,
					Obj:         "00000001a",
					headBlock:   tinyBlk("00000001a"),
					block:       tinyBlk("00000001a"),
					lastLIBSent: tinyBlk("00000001a"),
					StepCount:   1,
					StepBlocks: []*bstream.PreprocessedBlock{
						{Block: bTestBlock("00000001a", "00000000a"), Obj: "00000001a"},
					},
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000002a"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
			},
		},
		{

			// First Streamable Block
			name: "cursor step:irreversible first streamable",
			cursor: &bstream.Cursor{
				Step:      bstream.StepIrreversible,
				LIB:       bTestBlock("00000001a", "00000000a"),
				HeadBlock: bTestBlock("00000001a", "00000000a"),
				Block:     bTestBlock("00000001a", "00000000a"),
			},
			processBlocks: []*bstream.Block{
				bTestBlock("00000001a", "00000000a"),
				bTestBlock("00000002a", "00000001a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000002a"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
			},
		},

		{
			// Step:New Block:4a Head:5a LIB: 2a (caused by either reorg on 5a, or disordered blocks received 2a,4a,3a,5a)

			name: "cursor step:new advanced HEAD",
			cursor: &bstream.Cursor{
				Step:      bstream.StepNew,
				LIB:       bTestBlock("00000002a", "00000001a"),
				HeadBlock: bTestBlock("00000005a", "00000004a"),
				Block:     bTestBlock("00000004a", "00000003a"),
			},
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"),
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000004a", "00000003a"),
				bTestBlock("00000005a", "00000004a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000005a",
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000005a"),
					lastLIBSent: tinyBlk("00000002a"),
				},
			},
		},
		{
			// Step:New Block:4a Head:8a LIB: 2a (caused by either reorg on 8a, or disordered blocks received 2a,4a,5a,6a,7a,3a,8a)
			// possible issue: when we get 8a we don't have a complete chain yet (because of ordering, maybe)... after we see 8a, if it's incomplete, we keep gathering blocks for a while until we have a complete chain up to 8a

			name: "cursor step:new very advanced HEAD",
			cursor: &bstream.Cursor{
				Step:      bstream.StepNew,
				LIB:       bTestBlock("00000002a", "00000001a"),
				HeadBlock: bTestBlock("00000008a", "00000007a"),
				Block:     bTestBlock("00000004a", "00000003a"),
			},
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"),
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000004a", "00000003a"),
				bTestBlock("00000005a", "00000004a"),
				bTestBlock("00000006a", "00000005a"),
				bTestBlock("00000007a", "00000006a"),
				bTestBlock("00000008a", "00000007a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000005a",
					headBlock:   tinyBlk("00000008a"),
					block:       tinyBlk("00000005a"),
					lastLIBSent: tinyBlk("00000002a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000006a",
					headBlock:   tinyBlk("00000008a"),
					block:       tinyBlk("00000006a"),
					lastLIBSent: tinyBlk("00000002a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000007a",
					headBlock:   tinyBlk("00000008a"),
					block:       tinyBlk("00000007a"),
					lastLIBSent: tinyBlk("00000002a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000008a",
					headBlock:   tinyBlk("00000008a"),
					block:       tinyBlk("00000008a"),
					lastLIBSent: tinyBlk("00000002a"),
				},
			},
		},
		{
			// Step:Undo Block:5b Head:8a LIB: 2a (caused reorg on 8a after going up to 7b: 2a,3b,4b,5b,6b,7b,3a,4a,5a,6a,7a,8a))
			//   0. expect startBlock at 2
			//   1. set LIB 2a
			//   2. flow at 5b,
			//   3. (prevent sending anything or reorg-ing unless we are at head 8a, then force this to be the longest chain)
			//   4. open gate after we get undo:5b with head=8a
			// possible issue: when we get 8a we don't have a complete chain yet (because of ordering, maybe)... after we see 8a, if it's incomplete, we keep gathering blocks for a while until we have a complete chain up to 8a

			name: "cursor step:undo very advanced HEAD",
			cursor: &bstream.Cursor{
				Step:      bstream.StepUndo,
				LIB:       bTestBlock("00000002a", "00000001a"),
				HeadBlock: bTestBlock("00000008a", "00000007a"),
				Block:     bTestBlock("00000005b", "00000004a"),
			},
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"),
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000004a", "00000003a"),
				bTestBlock("00000005b", "00000004a"),
				bTestBlock("00000006b", "00000005b"),
				bTestBlock("00000007b", "00000006b"),
				bTestBlock("00000005a", "00000004a"),
				bTestBlock("00000006a", "00000005a"),
				bTestBlock("00000007a", "00000006a"),
				bTestBlock("00000008a", "00000007a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000005a",
					headBlock:   tinyBlk("00000008a"),
					block:       tinyBlk("00000005a"),
					lastLIBSent: tinyBlk("00000002a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000006a",
					headBlock:   tinyBlk("00000008a"),
					block:       tinyBlk("00000006a"),
					lastLIBSent: tinyBlk("00000002a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000007a",
					headBlock:   tinyBlk("00000008a"),
					block:       tinyBlk("00000007a"),
					lastLIBSent: tinyBlk("00000002a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000008a",
					headBlock:   tinyBlk("00000008a"),
					block:       tinyBlk("00000008a"),
					lastLIBSent: tinyBlk("00000002a"),
				},
			},
		},
		{

			// Step:Irreversible Block:4a Head:6a LIB: 4a
			//   0. expect startBlock at 4
			//   1. set LIB 4a
			//   2. ensureBlockFlow at 4a
			//   3. (prevent sending anything unless we are at head 6a, then force this to be the longest chain)
			//   4. open gate after we get new:6a // watchout, we will not see the actual step:Irr on block 4a :P
			// possible issue: when we get 6a we don't have a complete chain yet (because of ordering, maybe)... after we see 6a, if it's incomplete, we keep gathering blocks for a while until we have a complete chain up to 6a

			name: "cursor step:irr",
			cursor: &bstream.Cursor{
				Step:      bstream.StepIrreversible,
				LIB:       bTestBlock("00000004a", "00000003a"),
				Block:     bTestBlock("00000004a", "00000003a"),
				HeadBlock: bTestBlock("00000006a", "00000005a"),
			},
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"),
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000004a", "00000003a"),
				bTestBlock("00000005a", "00000004a"),
				tb("00000006a", "00000005a", 5),
				bTestBlock("00000007a", "00000006a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepIrreversible,
					Obj:         "00000005a",
					headBlock:   tinyBlk("00000006a"),
					block:       tinyBlk("00000005a"),
					lastLIBSent: tinyBlk("00000005a"),
					StepCount:   1,
					StepBlocks: []*bstream.PreprocessedBlock{
						{Block: bTestBlock("00000005a", "00000004a"), Obj: "00000005a"},
					},
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000007a",
					headBlock:   tinyBlk("00000007a"),
					block:       tinyBlk("00000007a"),
					lastLIBSent: tinyBlk("00000005a"),
				},
			},
		},
		{

			/*
				                             New(4b)
				                              /
				                             /
				NEW(1a) --> New(2a) --> New(3a) --> New(4a)

				Flow: New(1a) => New(2a) => New(3a) => New(4b) ==> Irr(1a) [ DROP ]
				Reconnect: .... Irr(2a) -> Undo(4b) -> New(4a)
			*/
			name: "cursor complex",
			cursor: &bstream.Cursor{
				Step: bstream.StepIrreversible,
				LIB:  tinyBlk("00000001a"), Block: tinyBlk("00000001a"), HeadBlock: tinyBlk("00000004b"),
			},
			processBlocks: []*bstream.Block{
				bTestBlock("00000001a", "00000000a"),
				bTestBlock("00000002a", "00000001a"),
				bTestBlock("00000003a", "00000002a"),
				tb("00000004b", "00000003a", 2),
				bTestBlock("00000004a", "00000003a"),
				bTestBlock("00000005a", "00000004a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepIrreversible,
					Obj:         "00000002a",
					block:       tinyBlk("00000002a"),
					headBlock:   tinyBlk("00000004b"),
					lastLIBSent: tinyBlk("00000002a"),
					StepCount:   1,
					StepBlocks: []*bstream.PreprocessedBlock{
						{Block: bTestBlock("00000002a", "00000001a"), Obj: "00000002a"},
					},
				},
				{
					step:        bstream.StepUndo,
					Obj:         "00000004b",
					block:       tinyBlk("00000004b"),
					headBlock:   tinyBlk("00000005a"),
					lastLIBSent: tinyBlk("00000002a"),
					StepCount:   1,
					StepIndex:   0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{tb("00000004b", "00000003a", 2), "00000004b"},
					},
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004a",
					block:       tinyBlk("00000004a"),
					headBlock:   tinyBlk("00000005a"),
					lastLIBSent: tinyBlk("00000002a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000005a",
					block:       tinyBlk("00000005a"),
					headBlock:   tinyBlk("00000005a"),
					lastLIBSent: tinyBlk("00000002a"),
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := newTestForkableSink(nil, nil)

			fap := New(bstream.TestChainConfig(), p, FromCursor(c.cursor))
			if c.filterSteps != 0 {
				fap.filterSteps = c.filterSteps
			}

			for _, res := range c.expectedResult {
				res.ForkDB = fap.forkDB
			}
			var err error
			for _, b := range c.processBlocks {
				err = fap.ProcessBlock(b, b.ID())
				require.NoError(t, err)
			}

			expected, err := json.MarshalIndent(c.expectedResult, "", "  ")
			require.NoError(t, err)
			result, err := json.MarshalIndent(p.results, "", "  ")
			require.NoError(t, err)

			if !assert.Equal(t, string(expected), string(result)) {
				fmt.Println("Expected: ", string(expected))
				fmt.Println("result: ", string(result))
			}
			require.Equal(t, len(c.expectedResult), len(p.results))
			for i := range c.expectedResult {
				expectedCursor := c.expectedResult[i].Cursor().String()
				actualCursor := p.results[i].Cursor().String()
				assert.Equal(t, expectedCursor, actualCursor, "cursors do not match")
			}

		})
	}
}

// testing cursor being applied...
// Note: in cursor and firehose, Redo is changed into New, only the different HeadBlock matters

func TestForkable_ProcessBlock(t *testing.T) {
	cases := []struct {
		name                               string
		forkDB                             *ForkDB
		ensureAllBlocksTriggerLongestChain bool
		ensureBlockFlows                   bstream.BlockRef
		includeInitialLIB                  bool
		filterSteps                        bstream.StepType
		processBlocks                      []*bstream.Block
		undoErr                            error
		redoErr                            error
		startBlock                         uint64
		expectedResultCount                int
		expectedResult                     []*ForkableObject
		expectedError                      string
		protocolFirstBlock                 uint64
	}{
		{
			name:               "inclusive enabled",
			forkDB:             fdbLinked(2, "00000003a"),
			protocolFirstBlock: 2,
			includeInitialLIB:  true,
			processBlocks: []*bstream.Block{
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000004a", "00000003a"), //StepNew 00000002a
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000003a"),
				},
				{
					step:        bstream.StepIrreversible,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000003a"), // artificially set in forkdb
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000003a"),
					StepCount:   1,
					StepIndex:   0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000003a", "00000002a"), "00000003a"},
					},
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004a",
					headBlock:   tinyBlk("00000004a"),
					block:       tinyBlk("00000004a"),
					lastLIBSent: tinyBlk("00000003a"),
				},
			},
		},
		{
			name:               "Test skip block",
			forkDB:             fdbLinked(2, "00000003a"),
			protocolFirstBlock: 2,
			includeInitialLIB:  true,
			processBlocks: []*bstream.Block{
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000005a", "00000003a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000003a"),
				},
				{
					step:        bstream.StepIrreversible,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000003a"), // artificially set in forkdb
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000003a"),
					StepCount:   1,
					StepIndex:   0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000003a", "00000002a"), "00000003a"},
					},
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000005a",
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000005a"),
					lastLIBSent: tinyBlk("00000003a"),
				},
			},
		},
		{
			name:               "inclusive disabled",
			forkDB:             fdbLinked(2, "00000003a"),
			protocolFirstBlock: 2,
			includeInitialLIB:  false,
			processBlocks: []*bstream.Block{
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000004a", "00000003a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000004a",
					headBlock:   tinyBlk("00000004a"),
					block:       tinyBlk("00000004a"),
					lastLIBSent: tinyBlk("00000003a"),
				},
			},
		},
		{
			name:               "cursor has LIB when irreversible never sent",
			forkDB:             fdbLinked(2, "00000003a"),
			protocolFirstBlock: 2,
			includeInitialLIB:  false,
			processBlocks: []*bstream.Block{
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000004a", "00000003a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000004a",
					headBlock:   tinyBlk("00000004a"),
					block:       tinyBlk("00000004a"),
					lastLIBSent: tinyBlk("00000003a"),
				},
			},
		},
		{
			name:   "undos redos and skip",
			forkDB: fdbLinked(2, "00000001a"),

			protocolFirstBlock: 2,
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"), //StepNew 00000002a
				bTestBlock("00000003a", "00000002a"), //StepNew 00000003a
				bTestBlock("00000003b", "00000002a"), //nothing
				bTestBlock("00000004b", "00000003b"), //StepUndo 00000003a, StepNew 00000003b, StepNew 00000004b
				bTestBlock("00000004a", "00000003a"), //nothing not longest chain
				bTestBlock("00000005a", "00000004a"), //StepUndo 00000004b, StepUndo 00000003b, StepRedo 00000003a, StepNew 00000004a
				bTestBlock("00000007a", "00000006a"), //nothing not longest chain
				bTestBlock("00000006a", "00000005a"), //StepNew 00000006a (not sending 7a yet, 6a does not trigger numbers above 6 in our algo)
				bTestBlock("00000008a", "00000007a"), //StepNew 00000007a, StepNew 00000008a
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000002a"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepUndo,
					Obj:         "00000003a",
					StepCount:   1,
					StepIndex:   0,
					headBlock:   tinyBlk("00000004b"), // cause of this
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000001a"),
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000003a", "00000002a"), "00000003a"},
					},
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000003b",
					headBlock:   tinyBlk("00000004b"),
					block:       tinyBlk("00000003b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004b",
					headBlock:   tinyBlk("00000004b"),
					block:       tinyBlk("00000004b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepUndo,
					Obj:         "00000004b",
					StepCount:   2,
					StepIndex:   0,
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000004b"),
					lastLIBSent: tinyBlk("00000001a"),
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000004b", "00000003b"), "00000004b"},
						{bTestBlock("00000003b", "00000002a"), "00000003b"},
					},
				},
				{
					step:        bstream.StepUndo,
					Obj:         "00000003b",
					StepCount:   2,
					StepIndex:   1,
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000003b"),
					lastLIBSent: tinyBlk("00000001a"),
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000004b", "00000003b"), "00000004b"},
						{bTestBlock("00000003b", "00000002a"), "00000003b"},
					},
				},
				{
					step:        bstream.StepRedo,
					Obj:         "00000003a",
					StepCount:   1,
					StepIndex:   0,
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000001a"),
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000003a", "00000002a"), "00000003a"},
					},
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004a",
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000004a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000005a",
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000005a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000006a",
					headBlock:   tinyBlk("00000006a"),
					block:       tinyBlk("00000006a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000007a",
					headBlock:   tinyBlk("00000008a"), // edge case, blocks were disordered so 7 comes with 8 as head
					block:       tinyBlk("00000007a"), // we may want to fake headBlock into 7 here FIXME
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000008a",
					headBlock:   tinyBlk("00000008a"),
					block:       tinyBlk("00000008a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
			},
		},
		{
			name:               "irreversible",
			forkDB:             fdbLinked(2, "00000001a"),
			protocolFirstBlock: 2,
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"), //StepNew 00000002a
				tb("00000003a", "00000002a", 2),      //StepNew 00000003a
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000002a"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepIrreversible,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000002a"),
					StepCount:   1,
					StepIndex:   0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000002a", "00000001a"), "00000002a"},
					},
				},
			},
		},
		{
			name:               "stalled",
			forkDB:             fdbLinked(2, "00000001a"),
			protocolFirstBlock: 2,
			processBlocks: []*bstream.Block{
				tb("00000002a", "00000001a", 1),
				tb("00000003a", "00000002a", 2),
				tb("00000003b", "00000002a", 2),
				tb("00000004a", "00000003a", 3),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000002a"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepIrreversible,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000002a"),
					StepCount:   1,
					StepIndex:   0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{tb("00000002a", "00000001a", 1), "00000002a"},
					},
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004a",
					headBlock:   tinyBlk("00000004a"),
					block:       tinyBlk("00000004a"),
					lastLIBSent: tinyBlk("00000002a"),
				},
				{
					step:        bstream.StepIrreversible,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000004a"),
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000003a"),
					StepCount:   1,
					StepIndex:   0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{tb("00000003a", "00000002a", 2), "00000003a"},
					},
				},
				{
					step:        bstream.StepStalled,
					Obj:         "00000003b",
					headBlock:   tinyBlk("00000004a"),
					block:       tinyBlk("00000003b"),
					lastLIBSent: tinyBlk("00000003a"),
					StepCount:   1,
					StepIndex:   0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{tb("00000003b", "00000002a", 2), "00000003b"},
					},
				},
			},
		},
		{
			name:               "undos error",
			forkDB:             fdbLinked(2, "00000001a"),
			protocolFirstBlock: 2,
			undoErr:            fmt.Errorf("error.1"),
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"), //StepNew 00000002a
				bTestBlock("00000003a", "00000002a"), //StepNew 00000003a
				bTestBlock("00000003b", "00000002a"), //nothing
				bTestBlock("00000004b", "00000003b"), //StepUndo 00000003a, StepNew 00000003b, StepNew 00000004b
			},
			expectedError:  "error.1",
			expectedResult: []*ForkableObject{},
		},
		{
			name:               "undos error with skip block",
			forkDB:             fdbLinked(2, "00000001a"),
			protocolFirstBlock: 2,
			undoErr:            fmt.Errorf("error.1"),
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"), //StepNew 00000002a
				bTestBlock("00000003a", "00000002a"), //StepNew 00000003a
				bTestBlock("00000004b", "00000002a"), //SKIPPING BLOCK 3B
				bTestBlock("00000005b", "00000004b"), //StepUndo 00000003a, StepNew 00000003b, StepNew 00000004b
			},
			expectedError:  "error.1",
			expectedResult: []*ForkableObject{},
		},
		{
			name:               "redos error",
			forkDB:             fdbLinked(2, "00000001a"),
			protocolFirstBlock: 2,
			redoErr:            fmt.Errorf("error.1"),
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"), //StepNew 00000002a
				bTestBlock("00000003a", "00000002a"), //StepNew 00000003a
				bTestBlock("00000003b", "00000002a"), //nothing
				bTestBlock("00000004b", "00000003b"), //StepUndo 00000003a, StepNew 00000003b, StepNew 00000004b
				bTestBlock("00000004a", "00000003a"), //nothing not longest chain
				bTestBlock("00000005a", "00000004a"), //StepUndo 00000004b, StepUndo 00000003b, StepRedos 00000003a, StepNew 00000004a
			},
			expectedError:  "error.1",
			expectedResult: []*ForkableObject{},
		},
		{
			name:               "redos error with skip block",
			forkDB:             fdbLinked(2, "00000001a"),
			protocolFirstBlock: 2,
			redoErr:            fmt.Errorf("error.1"),
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"), //StepNew 00000002a
				bTestBlock("00000003a", "00000002a"), //StepNew 00000003a
				bTestBlock("00000003b", "00000002a"), //nothing
				bTestBlock("00000004b", "00000003b"), //StepUndo 00000003a, StepNew 00000003b, StepNew 00000004b
				bTestBlock("00000005a", "00000003a"), //SKIPPING BLOCK 4A
				bTestBlock("00000006a", "00000005a"), //StepUndo 00000004b, StepUndo 00000003b, StepRedos 00000003a, StepNew 00000004a
			},
			expectedError:  "error.1",
			expectedResult: []*ForkableObject{},
		},
		{
			name:   "out of order block",
			forkDB: fdbLinked(2, "00000001a"),

			protocolFirstBlock: 2,
			processBlocks: []*bstream.Block{
				bTestBlock("00000003b", "00000002a"), //nothing
			},
			expectedResult: []*ForkableObject{},
		},
		{
			name:   "start with a fork!",
			forkDB: fdbLinked(1, "00000001a"),

			protocolFirstBlock: 1, // cannot use "2" here, starting on a WRONG firstStreamableBlock is not supported
			processBlocks: []*bstream.Block{
				bTestBlock("00000002b", "00000001a"), //StepNew 00000002a
				bTestBlock("00000002a", "00000001a"), //Nothing
				bTestBlock("00000003a", "00000002a"), //StepNew 00000002a, StepNew 00000003a
				bTestBlock("00000004a", "00000003a"), //StepNew 00000004a
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000002b",
					headBlock:   tinyBlk("00000002b"),
					block:       tinyBlk("00000002b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepUndo,
					Obj:         "00000002b",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000002b"),
					lastLIBSent: tinyBlk("00000001a"),
					StepCount:   1,
					StepIndex:   0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000002b", "00000001a"), "00000002b"},
					},
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004a",
					headBlock:   tinyBlk("00000004a"),
					block:       tinyBlk("00000004a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
			},
		},
		{
			name:   "start with a fork! with skip block",
			forkDB: fdbLinked(2, "00000001a"),
			protocolFirstBlock: 2,
			processBlocks: []*bstream.Block{
				bTestBlock("00000002b", "00000001a"), //StepNew 00000002a
				bTestBlock("00000002a", "00000001a"), //Nothing
				bTestBlock("00000003a", "00000002a"), //StepNew 00000002a, StepNew 00000003a
				bTestBlock("00000005a", "00000003a"), //StepNew 00000004a
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000002b",
					headBlock:   tinyBlk("00000002b"),
					block:       tinyBlk("00000002b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepUndo,
					Obj:         "00000002b",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000002b"),
					lastLIBSent: tinyBlk("00000001a"),
					StepCount:   1,
					StepIndex:   0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000002b", "00000001a"), "00000002b"},
					},
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000005a",
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000005a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
			},
		},
		{
			name:                               "ensure all blocks are new",
			ensureAllBlocksTriggerLongestChain: true,
			filterSteps:                        bstream.StepNew | bstream.StepIrreversible,
			forkDB:                             fdbLinked(2, "00000001a"),
			protocolFirstBlock:                 2,
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"),
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000003b", "00000002a"),
				bTestBlock("00000004b", "00000003b"),
				bTestBlock("00000004a", "00000003a"),
				bTestBlock("00000002b", "00000001a"),
				tb("00000005b", "00000004b", 3),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000002a"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000003b",
					headBlock:   tinyBlk("00000003b"),
					block:       tinyBlk("00000003b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004b",
					headBlock:   tinyBlk("00000004b"),
					block:       tinyBlk("00000004b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004a",
					headBlock:   tinyBlk("00000004a"),
					block:       tinyBlk("00000004a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000002b",
					headBlock:   tinyBlk("00000002b"),
					block:       tinyBlk("00000002b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000005b",
					headBlock:   tinyBlk("00000005b"),
					block:       tinyBlk("00000005b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepIrreversible,
					headBlock:   tinyBlk("00000005b"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000002a"),
					Obj:         "00000002a",
					StepCount:   2,
					StepIndex:   0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000002a", "00000001a"), "00000002a"},
						{bTestBlock("00000003b", "00000002a"), "00000003b"},
					},
				},
				{
					step:        bstream.StepIrreversible,
					Obj:         "00000003b",
					headBlock:   tinyBlk("00000005b"),
					block:       tinyBlk("00000003b"),
					lastLIBSent: tinyBlk("00000003b"),
					StepCount:   2,
					StepIndex:   1,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000002a", "00000001a"), "00000002a"},
						{bTestBlock("00000003b", "00000002a"), "00000003b"},
					},
				},
			},
		},
		{
			name:                               "ensure all blocks are new, skip block",
			forkDB:                             fdbLinked(2, "00000001a"),
			ensureAllBlocksTriggerLongestChain: true,
			filterSteps:                        bstream.StepNew | bstream.StepIrreversible,
			protocolFirstBlock:                 2,
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"),
				bTestBlock("00000004a", "00000002a"), //SKIPPING BLOCK 3a
				bTestBlock("00000004b", "00000002a"), //SKIPPING BLOCK 3b
				bTestBlock("00000005b", "00000004b"),
				bTestBlock("00000005a", "00000004a"),
				bTestBlock("00000002b", "00000001a"),
				tb("00000006b", "00000005b", 3),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000002a"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004a",
					headBlock:   tinyBlk("00000004a"),
					block:       tinyBlk("00000004a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004b",
					headBlock:   tinyBlk("00000004b"),
					block:       tinyBlk("00000004b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000005b",
					headBlock:   tinyBlk("00000005b"),
					block:       tinyBlk("00000005b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000005a",
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000005a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000002b",
					headBlock:   tinyBlk("00000002b"),
					block:       tinyBlk("00000002b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000006b",
					headBlock:   tinyBlk("00000006b"),
					block:       tinyBlk("00000006b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepIrreversible,
					headBlock:   tinyBlk("00000006b"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000002a"),
					Obj:         "00000002a",
					StepCount:   2,
					StepIndex:   0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000002a", "00000001a"), "00000002a"},
						{bTestBlock("00000004b", "00000002a"), "00000004b"},
					},
				},
				{
					step:        bstream.StepIrreversible,
					Obj:         "00000004b",
					headBlock:   tinyBlk("00000006b"),
					block:       tinyBlk("00000004b"),
					lastLIBSent: tinyBlk("00000004b"),
					StepCount:   2,
					StepIndex:   1,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000002a", "00000001a"), "00000002a"},
						{bTestBlock("00000004b", "00000002a"), "00000004b"},
					},
				},
			},
		},
		{
			name:                               "ensure all blocks are new with no holes",
			forkDB:                             fdbLinked(2, "00000001a"),
			ensureAllBlocksTriggerLongestChain: true,
			filterSteps:                        bstream.StepNew,
			protocolFirstBlock:                 2,
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"),
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000003b", "00000002a"),
				bTestBlock("00000004b", "00000003b"),
				bTestBlock("00000004a", "00000003a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000002a"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000003b",
					headBlock:   tinyBlk("00000003b"),
					block:       tinyBlk("00000003b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004b",
					headBlock:   tinyBlk("00000004b"),
					block:       tinyBlk("00000004b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004a",
					headBlock:   tinyBlk("00000004a"),
					block:       tinyBlk("00000004a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
			},
		},
		{
			name:                               "ensure all blocks are new with holes skips some forked blocks",
			forkDB:                             fdbLinked(2, "00000001a"),
			ensureAllBlocksTriggerLongestChain: true,
			filterSteps:                        bstream.StepNew,
			protocolFirstBlock:                 2,
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"),
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000004b", "00000003b"),
				bTestBlock("00000003b", "00000002a"),
				bTestBlock("00000004a", "00000003a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000002a"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000003a"),
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				// {  // not there, because there was a hole in here.. :deng:
				// 	Step: bstream.StepNew,
				// 	Obj:  "00000004b",
				// },
				{
					step:        bstream.StepNew,
					Obj:         "00000003b",
					headBlock:   tinyBlk("00000003b"),
					block:       tinyBlk("00000003b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004a",
					headBlock:   tinyBlk("00000004a"),
					block:       tinyBlk("00000004a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
			},
		},
		{
			name:               "ensure block ID goes through preceded by hole",
			forkDB:             fdbLinked(2, "00000001a"),
			ensureBlockFlows:   bRef("00000004b"),
			filterSteps:        bstream.StepNew | bstream.StepUndo | bstream.StepRedo,
			protocolFirstBlock: 2,
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"),
				bTestBlock("00000004b", "00000003a"),
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000004a", "00000003a"),
				bTestBlock("00000005a", "00000004a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000004b"), // nothing before that one
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000004b"),
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004b",
					headBlock:   tinyBlk("00000004b"),
					block:       tinyBlk("00000004b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepUndo,
					Obj:         "00000004b",
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000004b"),
					lastLIBSent: tinyBlk("00000001a"),
					StepCount:   1,
					StepIndex:   0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000004b", "00000003a"), "00000004b"},
					},
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004a",
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000004a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000005a",
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000005a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
			},
		},
		{
			name:               "ensure block ID goes through",
			forkDB:             fdbLinked(2, "00000001a"),
			ensureBlockFlows:   bRef("00000003b"),
			protocolFirstBlock: 2,
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"),
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000004a", "00000003a"),
				bTestBlock("00000003b", "00000002a"),
				bTestBlock("00000005a", "00000004a"),
				bTestBlock("00000002b", "00000001a"),
			},
			expectedResult: []*ForkableObject{
				{
					step:        bstream.StepNew,
					Obj:         "00000002a",
					headBlock:   tinyBlk("00000003b"),
					block:       tinyBlk("00000002a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000003b",
					headBlock:   tinyBlk("00000003b"),
					block:       tinyBlk("00000003b"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepUndo,
					Obj:         "00000003b",
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000003b"),
					lastLIBSent: tinyBlk("00000001a"),
					StepCount:   1,
					StepIndex:   0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000003b", "00000002a"), "00000003b"},
					},
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000003a",
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000003a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000004a",
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000004a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
				{
					step:        bstream.StepNew,
					Obj:         "00000005a",
					headBlock:   tinyBlk("00000005a"),
					block:       tinyBlk("00000005a"),
					lastLIBSent: tinyBlk("00000001a"),
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := newTestForkableSink(c.undoErr, c.redoErr)
			chainConfig := bstream.TestChainConfig()
			chainConfig.FirstStreamableBlock = c.protocolFirstBlock

			fap := New(chainConfig, p)
			fap.forkDB = c.forkDB
			if fap.forkDB.HasLIB() {
				fap.lastLIBSeen = fap.forkDB.libRef
			}
			fap.ensureAllBlocksTriggerLongestChain = c.ensureAllBlocksTriggerLongestChain
			fap.includeInitialLIB = c.includeInitialLIB

			if c.ensureBlockFlows != nil {
				fap.ensureBlockFlows = c.ensureBlockFlows
			}

			if c.filterSteps != 0 {
				fap.filterSteps = c.filterSteps
			}

			var err error
			for _, b := range c.processBlocks {
				err = fap.ProcessBlock(b, b.ID())
			}
			if c.expectedError != "" {
				require.True(t, strings.HasSuffix(err.Error(), c.expectedError))
				return
			}

			for _, res := range c.expectedResult {
				res.ForkDB = c.forkDB
			}

			expected, err := json.MarshalIndent(c.expectedResult, "", "  ")
			require.NoError(t, err)
			result, err := json.MarshalIndent(p.results, "", "  ")
			require.NoError(t, err)

			// _ = expected
			// _ = result
			if !assert.Equal(t, string(expected), string(result)) {
				fmt.Println("Expected: ", string(expected))
				fmt.Println("result: ", string(result))
			}
			require.Equal(t, len(c.expectedResult), len(p.results))
			for i := range c.expectedResult {
				expectedCursor := c.expectedResult[i].Cursor().String()
				actualCursor := p.results[i].Cursor().String()
				assert.Equal(t, expectedCursor, actualCursor, "cursors do not match")
			}

		})
	}
}

func TestForkable_ProcessBlock_UnknownLIB(t *testing.T) {
	cases := []struct {
		name                string
		forkDB              *ForkDB
		processBlocks       []*bstream.Block
		protocolFirstBlock  uint64
		undoErr             error
		redoErr             error
		expectedResultCount int
		expectedResult      []*ForkableObject
		expectedError       string
		expectedCursors     []string // optional
		libnumGetter        LIBNumGetter
	}{
		{
			name:               "Expecting block 1 (Ethereum test case)",
			forkDB:             fdbLinkedWithoutLIB(1),
			protocolFirstBlock: 1,
			processBlocks: []*bstream.Block{
				tb("00000001a", "00000000a", 1), //this is to replicate the bad behaviour of LIBNum() of codec/deth.go
				//bTestBlock("00000002a", "00000001a"), //bstream.StepNew 00000002a
			},
			expectedResult: []*ForkableObject{
				{
					step: bstream.StepNew,
					Obj:  "00000001a",
				},
				{
					step:      bstream.StepIrreversible,
					Obj:       "00000001a",
					StepCount: 1,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{tb("00000001a", "00000000a", 1), "00000001a"},
					},
				},
			},
			expectedCursors: []string{
				"c1:1:1:00000001a:1:00000001a",
				"c1:16:1:00000001a:1:00000001a",
			},
		},
		{
			name:               "undos redos and skip",
			forkDB:             fdbLinkedWithoutLIB(1),
			protocolFirstBlock: 2,
			processBlocks: []*bstream.Block{
				tb("00000002a", "00000001a", 1),      //StepNew 00000002a
				bTestBlock("00000003a", "00000002a"), //StepNew 00000003a
				bTestBlock("00000003b", "00000002a"), //nothing
				bTestBlock("00000004b", "00000003b"), //StepUndo 00000003a, StepNew 00000003b, StepNew 00000004b
				bTestBlock("00000004a", "00000003a"), //nothing not longest chain
				bTestBlock("00000005a", "00000004a"), //StepUndo 00000004b, StepUndo 00000003b, StepRedo 00000003a, StepNew 00000004a
				bTestBlock("00000007a", "00000006a"), //nothing not longest chain
				bTestBlock("00000006a", "00000005a"), //nothing
				bTestBlock("00000008a", "00000007a"), //StepNew 00000007a, StepNew 00000008a
			},
			expectedResult: []*ForkableObject{
				{
					step: bstream.StepNew,
					Obj:  "00000002a",
				},
				{
					step:      bstream.StepIrreversible,
					Obj:       "00000002a",
					StepCount: 1,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{tb("00000002a", "00000001a", 1), "00000002a"},
					},
				},
				{
					step: bstream.StepNew,
					Obj:  "00000003a",
				},
				{
					step:      bstream.StepUndo,
					Obj:       "00000003a",
					StepCount: 1,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000003a", "00000002a"), "00000003a"},
					},
				},
				{
					step: bstream.StepNew,
					Obj:  "00000003b",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000004b",
				},
				{
					step:      bstream.StepUndo,
					Obj:       "00000004b",
					StepCount: 2,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000004b", "00000003b"), "00000004b"},
						{bTestBlock("00000003b", "00000002a"), "00000003b"},
					},
				},
				{
					step:      bstream.StepUndo,
					Obj:       "00000003b",
					StepCount: 2,
					StepIndex: 1,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000004b", "00000003b"), "00000004b"},
						{bTestBlock("00000003b", "00000002a"), "00000003b"},
					},
				},
				{
					step:      bstream.StepRedo,
					Obj:       "00000003a",
					StepCount: 1,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000003a", "00000002a"), "00000003a"},
					},
				},
				{
					step: bstream.StepNew,
					Obj:  "00000004a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000005a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000006a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000007a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000008a",
				},
			},
		},
		{
			name:               "irreversible",
			forkDB:             fdbLinkedWithoutLIB(1),
			protocolFirstBlock: 2,
			processBlocks: []*bstream.Block{
				tb("00000002a", "00000001a", 1), //StepNew 00000002a, StepIrreversible 2a (firstStreamable)
				tb("00000003a", "00000002a", 2), //StepNew 00000003a  (2 already irr)
				tb("00000004a", "00000003a", 3), //StepNew 00000004a, StepIrreversible 3a
			},
			expectedResult: []*ForkableObject{
				{
					step: bstream.StepNew,
					Obj:  "00000002a",
				},
				{
					step:      bstream.StepIrreversible,
					Obj:       "00000002a",
					StepCount: 1,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{tb("00000002a", "00000001a", 1), "00000002a"},
					},
				},
				{
					step: bstream.StepNew,
					Obj:  "00000003a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000004a",
				},
				{
					step:      bstream.StepIrreversible,
					Obj:       "00000003a",
					StepCount: 1,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{tb("00000003a", "00000002a", 2), "00000003a"},
					},
				},
			},
		},
		{
			name:   "stalled",
			forkDB: fdbLinkedWithoutLIB(1),
			protocolFirstBlock: 2,
			processBlocks: []*bstream.Block{
				tb("00000002a", "00000001a", 1),
				tb("00000003a", "00000002a", 2),
				tb("00000003b", "00000002a", 2),
				tb("00000004a", "00000003a", 3),
			},
			expectedResult: []*ForkableObject{
				{
					step: bstream.StepNew,
					Obj:  "00000002a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000003a",
				},
				{
					step:      bstream.StepIrreversible,
					Obj:       "00000002a",
					StepCount: 1,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{tb("00000002a", "00000001a", 1), "00000002a"},
					},
				},
				{
					step: bstream.StepNew,
					Obj:  "00000004a",
				},
				{
					step:      bstream.StepIrreversible,
					Obj:       "00000003a",
					StepCount: 1,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{tb("00000003a", "00000002a", 2), "00000003a"},
					},
				},
				{
					step:      bstream.StepStalled,
					Obj:       "00000003b",
					StepCount: 1,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{tb("00000003b", "00000002a", 2), "00000003b"},
					},
				},
			},
		},
		{
			name:               "undos error",
			forkDB:             fdbLinkedWithoutLIB(2),
			protocolFirstBlock: 2,
			undoErr:            fmt.Errorf("error.1"),
			processBlocks: []*bstream.Block{
				tb("00000002a", "00000001a", 1),
				tb("00000003a", "00000002a", 1),
				tb("00000003b", "00000002a", 1),
				tb("00000004b", "00000003b", 1),
			},
			expectedError:  "error.1",
			expectedResult: []*ForkableObject{},
		},
		{
			name:               "redos error",
			forkDB:             fdbLinkedWithoutLIB(2),
			protocolFirstBlock: 2,
			redoErr:            fmt.Errorf("error.1"),
			processBlocks: []*bstream.Block{
				tb("00000002a", "00000001a", 1), //StepNew 00000002a
				tb("00000003a", "00000002a", 1), //StepNew 00000003a
				tb("00000003b", "00000002a", 1), //nothing
				tb("00000004b", "00000003b", 1), //StepUndo 00000003a, StepNew 00000003b, StepNew 00000004b
				tb("00000004a", "00000003a", 1), //nothing not longest chain
				tb("00000005a", "00000004a", 1), //StepUndo 00000004b, StepUndo 00000003b, StepRedos 00000003a, StepNew 00000004a
			},
			expectedError:  "error.1",
			expectedResult: []*ForkableObject{},
		},
		{
			name:               "out of order block",
			forkDB:             fdbLinkedWithoutLIB(2),
			protocolFirstBlock: 2,
			processBlocks: []*bstream.Block{
				bTestBlock("00000003b", "00000002a"), //nothing
			},
			expectedResult: []*ForkableObject{{
				step: bstream.StepNew,
				Obj:  "00000003b",
			},
			},
		},
		{
			name:               "start with a fork!",
			forkDB:             fdbLinkedWithoutLIB(1),
			protocolFirstBlock: 1,
			processBlocks: []*bstream.Block{
				tb("00000002b", "00000001a", 1), //StepNew 00000002a
				tb("00000002a", "00000001a", 1), //Nothing
				tb("00000003a", "00000002a", 1), //StepNew 00000002a, StepNew 00000003a, StepIrreversible 2a
				tb("00000004a", "00000003a", 1), //StepNew 00000004a
			},
			expectedResult: []*ForkableObject{
				{
					step: bstream.StepNew,
					Obj:  "00000002b",
				},
				{
					step:      bstream.StepUndo,
					Obj:       "00000002b",
					StepCount: 1,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{tb("00000002b", "00000001a", 1), "00000002b"},
					},
				},
				{
					step: bstream.StepNew,
					Obj:  "00000002a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000003a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000004a",
				},
			},
		},
		//{
		//	name:               "validate cannot go up to dposnum to set lib",
		//	forkDB:             fdbLinkedWithoutLIB(),
		//	protocolFirstBlock: 2,
		//	processBlocks: []*bstream.Block{
		//		tb("00000004a", "00000003a", 1),
		//		tb("00000005a", "00000004a", 2),
		//	},
		//	expectedResult: []*ForkableObject{},
		//},
		{
			name:               "validate we can set LIB to ID referenced as Previous and start sending after",
			forkDB:             fdbLinkedWithoutLIB(2),
			protocolFirstBlock: 2,
			processBlocks: []*bstream.Block{
				tb("00000003b", "00000002a", 1),
				tb("00000003a", "00000002a", 1),
				tb("00000004a", "00000003a", 2),
				tb("00000005a", "00000004a", 2),
			},
			expectedResult: []*ForkableObject{
				{
					step: bstream.StepNew,
					Obj:  "00000003b",
				},
				{
					step:      bstream.StepUndo,
					Obj:       "00000003b",
					StepCount: 1,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{tb("00000003b", "00000002a", 1), "00000003b"},
					},
				},
				{
					step: bstream.StepNew,
					Obj:  "00000003a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000004a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000005a",
				},
			},
		},
		{
			name:               "validate we can set LIB to ID actually seen and start sending after, with burst",
			forkDB:             fdbLinkedWithoutLIB(2),
			protocolFirstBlock: 2,
			processBlocks: []*bstream.Block{
				tb("00000003a", "00000002a", 1),
				tb("00000004a", "00000003a", 1),
				tb("00000004b", "00000003a", 1),
				tb("00000005a", "00000004a", 3),
			},
			expectedResult: []*ForkableObject{
				{
					step: bstream.StepNew,
					Obj:  "00000003a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000004a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000005a",
				},
				{
					step:      bstream.StepIrreversible,
					Obj:       "00000003a",
					StepCount: 1,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{tb("00000003a", "00000002a", 1), "00000003a"},
					},
				},
			},
		},
		{
			name:               "irreversible custom libnum getter",
			forkDB:             fdbLinkedWithoutLIB(2),
			protocolFirstBlock: 2,
			libnumGetter:       RelativeLIBNumGetter(1, 3),
			processBlocks: []*bstream.Block{
				bTestBlock("00000002a", "00000001a"), // sends 2a as irreversible (first streamable block)
				bTestBlock("00000003a", "00000002a"),
				bTestBlock("00000004a", "00000003a"),
				bTestBlock("00000005a", "00000004a"),
				bTestBlock("00000006a", "00000005a"),
			},

			expectedResult: []*ForkableObject{
				{
					step: bstream.StepNew,
					Obj:  "00000002a",
				},
				{
					step:      bstream.StepIrreversible,
					Obj:       "00000002a",
					StepCount: 1,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000002a", "00000001a"), "00000002a"},
					},
				},
				{
					step: bstream.StepNew,
					Obj:  "00000003a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000004a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000005a",
				},
				{
					step: bstream.StepNew,
					Obj:  "00000006a",
				},
				{
					step:      bstream.StepIrreversible,
					Obj:       "00000003a",
					StepCount: 1,
					StepIndex: 0,
					StepBlocks: []*bstream.PreprocessedBlock{
						{bTestBlock("00000003a", "00000002a"), "00000003a"},
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sinkHandle := newTestForkableSink(c.undoErr, c.redoErr)
			chainConfig := bstream.TestChainConfig()
			chainConfig.FirstStreamableBlock = c.protocolFirstBlock

			fap := New(chainConfig, sinkHandle)
			fap.forkDB = c.forkDB
			if c.libnumGetter != nil {
				fap.libnumGetter = c.libnumGetter
			}
			if fap.forkDB.HasLIB() {
				fap.lastLIBSeen = fap.forkDB.libRef
			}

			var err error
			for _, b := range c.processBlocks {
				err = fap.ProcessBlock(b, b.ID())
				if err != nil {
					break
				}
			}
			if c.expectedError != "" {
				require.Error(t, err)
				require.True(t, strings.HasSuffix(err.Error(), c.expectedError))
				return
			} else {
				require.NoError(t, err)
			}

			for _, res := range c.expectedResult {
				res.ForkDB = c.forkDB
			}

			expected, err := json.Marshal(c.expectedResult)
			require.NoError(t, err)
			result, err := json.Marshal(sinkHandle.results)
			require.NoError(t, err)

			if len(c.expectedCursors) > 0 {
				require.Equal(t, len(c.expectedCursors), len(sinkHandle.results))
				for i, res := range sinkHandle.results {
					assert.Equal(t, c.expectedCursors[i], res.Cursor().String())
				}
			}
			_ = expected
			_ = result
			if !assert.Equal(t, string(expected), string(result)) {
				fmt.Println("Expected: ", string(expected))
				fmt.Println("result: ", string(result))
			}
		})
	}
}

func TestRelativeLIBNumGetter(t *testing.T) {
	cases := []struct {
		name            string
		confirmations   uint64
		in              uint64
		expectedOut     uint64
		firstStreamable uint64
	}{
		{
			name:        "vanilla",
			in:          10,
			expectedOut: 7,

			confirmations:   3,
			firstStreamable: 2,
		},
		{
			name:        "firststreamable",
			in:          2,
			expectedOut: 2,

			confirmations:   10,
			firstStreamable: 2,
		},
		{
			name:        "aboveFirstStreamable",
			in:          4,
			expectedOut: 2,

			confirmations:   10,
			firstStreamable: 2,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := RelativeLIBNumGetter(c.firstStreamable, c.confirmations)
			out := g(bstream.NewBlockRef("", c.in), nil)
			assert.Equal(t, c.expectedOut, out)

		})
	}
}

func TestForkable_ForkDBContainsPreviousPreprocessedBlockObjects(t *testing.T) {
	var nilHandler bstream.Handler
	p := New(bstream.TestChainConfig(), nilHandler, WithExclusiveLIB(bRef("00000003a")))

	err := p.ProcessBlock(bTestBlock("00000004a", ""), "mama")
	require.NoError(t, err)

	blk := p.forkDB.BlockForID("00000004a")
	assert.Equal(t, "mama", blk.Object.(*ForkableBlock).Obj)
}

func TestComputeNewLongestChain(t *testing.T) {
	p := &Forkable{
		forkDB:           NewForkDB(1),
		ensureBlockFlows: bstream.BlockRefEmpty,
	}

	p.forkDB.MoveLIB(bRef("00000001a"))

	p.forkDB.AddLink(bRef("00000002a"), "00000001a", simplePpBlock("00000002a", "00000001a"))
	longestChain := p.computeNewLongestChain(simplePpBlock("00000002a", "00000001a"))
	expected := []*Block{
		simpleFdbBlock("00000002a", "00000001a"),
	}
	assert.Equal(t, expected, longestChain, "initial computing of longest chain")

	p.forkDB.AddLink(bRef("00000003a"), "00000002a", simplePpBlock("00000003a", "00000002a"))
	longestChain = p.computeNewLongestChain(simplePpBlock("00000003a", "00000002a"))
	expected = []*Block{
		simpleFdbBlock("00000002a", "00000001a"),
		simpleFdbBlock("00000003a", "00000002a"),
	}
	assert.Equal(t, expected, longestChain, "adding a block to longest chain computation")

	p.forkDB.MoveLIB(bRef("00000003a"))
	p.forkDB.AddLink(bRef("00000004a"), "00000003a", simplePpBlock("00000004a", "00000003a"))
	longestChain = p.computeNewLongestChain(simplePpBlock("00000004a", "00000003a"))
	expected = []*Block{
		simpleFdbBlock("00000004a", "00000003a"),
	}
	assert.Equal(t, expected, longestChain, "recalculating longest chain if lib changed")
}

func simplePpBlock(id, previous string) *ForkableBlock {
	return &ForkableBlock{
		Block: bTestBlock(id, previous),
	}
}

func simpleFdbBlock(id, previous string) *Block {
	return &Block{
		BlockID:  id,
		BlockNum: blocknum(id),
		Object:   simplePpBlock(id, previous),
	}
}

func blocknum(blockID string) uint64 {
	b := blockID
	if len(blockID) < 8 { // shorter version, like 8a for 00000008a
		b = fmt.Sprintf("%09s", blockID)
	}
	bin, err := hex.DecodeString(b[:8])
	if err != nil {
		return 0
	}
	return uint64(binary.BigEndian.Uint32(bin))
}

func TestForkableSentChainSwitchSegments(t *testing.T) {
	p := &Forkable{
		forkDB:           NewForkDB(1),
		ensureBlockFlows: bstream.BlockRefEmpty,
	}
	p.forkDB.AddLink(bRef("00000003a"), "00000002a", nil)
	p.forkDB.AddLink(bRef("00000002a"), "00000001a", nil)

	undos, redos := p.sentChainSwitchSegments(zlog, "00000003a", "00000003a")
	assert.Nil(t, undos)
	assert.Nil(t, redos)
}
