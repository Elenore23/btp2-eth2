package eth2

import (
	"bytes"
	"fmt"
	"strconv"
	"testing"

	"github.com/attestantio/go-eth2-client/spec/altair"
	"github.com/ethereum/go-ethereum/common"
	etypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	ssz "github.com/ferranbt/fastssz"
	"github.com/icon-project/btp2/common/codec"
	"github.com/icon-project/btp2/common/link"
	"github.com/icon-project/btp2/common/log"
	"github.com/icon-project/btp2/common/types"
	"github.com/stretchr/testify/assert"
)

func newReceiver(src, dest types.BtpAddress) *receiver {
	r := NewReceiver(
		src,
		dest,
		"https://sepolia.infura.io/v3/ffbf8ebe228f4758ae82e175640275e0",
		map[string]interface{}{
			"consensus_endpoint": "http://20.20.5.191:9596",
		},
		log.WithFields(log.Fields{log.FieldKeyPrefix: "test"}),
	)
	return r.(*receiver)
}

func TestReceiver_BlockProof(t *testing.T) {
	r := newReceiver(
		types.BtpAddress("btp://0xaa36a7.eth/0x11167e875E08a113706e8bA3010ac37329b0E6b2"),
		types.BtpAddress("btp://0x42.icon/cx8642ab29e608915b43e677d9bcb17ec902b4ec8b"),
	)
	defer r.Stop()

	// get Header
	tests := []struct {
		name string
		diff int64
	}{
		{
			name: "with blockRoots",
			diff: 101,
		},
		// TODO add
		//{
		//	name: "with historicalRoots",
		//	diff: 10000,
		//},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// get finalized header and set bls
			fu, err := r.cl.LightClientFinalityUpdate()
			assert.NoError(t, err)

			finalizedSlot := int64(fu.FinalizedHeader.Beacon.Slot)
			bls := &types.BMCLinkStatus{}
			bls.Verifier.Height = finalizedSlot

			// get header and set mp
			blockProofSlot := finalizedSlot - tt.diff
			header, err := r.cl.BeaconBlockHeader(strconv.FormatInt(blockProofSlot, 10))
			assert.NoError(t, err)

			mp := &messageProofData{
				Slot: blockProofSlot,
				Header: &altair.LightClientHeader{
					Beacon: header.Header.Message,
				},
			}

			// get BlockProof
			bp, err := r.blockProofForMessageProof(bls, mp)
			assert.NoError(t, err)

			// verify BlockProof
			assert.Equal(t, link.TypeBlockProof, bp.Type())
			assert.Equal(t, blockProofSlot, bp.ProofHeight())
			bpd := new(blockProofData)
			_, err = codec.RLP.UnmarshalFromBytes(bp.(*BlockProof).Payload(), bpd)
			assert.NoError(t, err)
			// bp.proof.leaf == hash_tree_root(bp.header)
			root, err := bpd.Header.Beacon.HashTreeRoot()
			assert.NoError(t, err)
			assert.Equal(t, root[:], bpd.Proof.Leaf)
			// ssz verify proof
			ok, err := ssz.VerifyProof(fu.FinalizedHeader.Beacon.StateRoot[:], bpd.Proof)
			assert.True(t, ok)
			assert.NoError(t, err)
		})
	}
}

func TestReceiver_MessageProof(t *testing.T) {
	slot := int64(2091171)
	r := newReceiver(
		types.BtpAddress("btp://0xaa36a7.eth/0x11167e875E08a113706e8bA3010ac37329b0E6b2"),
		types.BtpAddress("btp://0x42.icon/cx8642ab29e608915b43e677d9bcb17ec902b4ec8b"),
	)
	defer r.Stop()

	bh, err := r.cl.BeaconBlockHeader(strconv.FormatInt(slot, 10))
	assert.NoError(t, err)
	header := &altair.LightClientHeader{
		Beacon: bh.Header.Message,
	}

	var mp *messageProofData
	mp, err = r.makeMessageProofData(header)
	assert.NoError(t, err)

	// verify receiptsRoot
	ok, err := ssz.VerifyProof(bh.Header.Message.StateRoot[:], mp.ReceiptsRootProof)
	assert.True(t, ok)
	assert.NoError(t, err)

	block, err := r.cl.BeaconBlock(fmt.Sprintf("%d", slot))
	assert.NoError(t, err)
	assert.Equal(
		t,
		block.Capella.Message.Body.ExecutionPayload.ReceiptsRoot[:],
		mp.ReceiptsRootProof.Leaf[:],
	)

	// verify receipt
	receiptsRoot := common.BytesToHash(mp.ReceiptsRootProof.Leaf)
	for _, rp := range mp.ReceiptProofs {
		nl := new(light.NodeList)
		err = rlp.DecodeBytes(rp.Proof, nl)
		assert.NoError(t, err)
		value, err := trie.VerifyProof(
			receiptsRoot,
			rp.Key,
			nl.NodeSet(),
		)
		assert.NoError(t, err)
		var idx uint64
		err = rlp.DecodeBytes(rp.Key, &idx)
		assert.NoError(t, err)

		// check receipt
		receipt, err := receiptFromBytes(value)
		assert.NoError(t, err)
		find := false
		for _, l := range receipt.Logs {
			if bytes.Compare(l.Topics[0][:], r.fq.Topics[0][0][:]) == 0 {
				find = true
				break
			}
		}
		assert.True(t, find)
	}
}

func receiptFromBytes(bs []byte) (*etypes.Receipt, error) {
	r := new(etypes.Receipt)
	if err := r.UnmarshalBinary(bs); err != nil {
		return nil, err
	}
	return r, nil
}