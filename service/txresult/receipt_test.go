package txresult

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/icon-project/goloop/common"
	"github.com/icon-project/goloop/common/codec"
	"github.com/icon-project/goloop/common/db"
	"github.com/icon-project/goloop/module"
	"github.com/icon-project/goloop/server/jsonrpc"
)

func TestReceipt_JSON(t *testing.T) {
	addr := common.NewAddressFromString("cx0000000000000000000000000000000000000001")
	r := NewReceipt(addr)
	r.SetResult(module.StatusSuccess, big.NewInt(100), big.NewInt(1000), nil)
	r.SetCumulativeStepUsed(big.NewInt(100))
	jso, err := r.ToJSON(jsonrpc.APIVersionLast)
	if err != nil {
		t.Errorf("Fail on ToJSON err=%+v", err)
	}
	jb, err := json.MarshalIndent(jso, "", "    ")

	fmt.Printf("JSON: %s\n", jb)

	r2, err := NewReceiptFromJSON(jb, jsonrpc.APIVersionLast)
	if err != nil {
		t.Errorf("Fail on Making Receipt from JSON err=%+v", err)
		return
	}
	if !bytes.Equal(r.Bytes(), r2.Bytes()) {
		t.Errorf("Different bytes from Unmarshaled Receipt")
	}

	t.Logf("Encoded: % X", r.Bytes())

	r3 := new(receipt)
	err = r3.Reset(db.NewMapDB(), r.Bytes())
	assert.NoError(t, err)
	assert.Equal(t, r3.Bytes(), r2.Bytes())
}

func Test_EventLog_BytesEncoding(t *testing.T) {
	var ev eventLog

	ev.eventLogData.Addr.SetTypeAndID(false, []byte{0x02})
	ev.eventLogData.Indexed = [][]byte{
		[]byte("Test(int)"),
		[]byte{0x01},
	}
	ev.eventLogData.Data = nil

	evj, err := ev.ToJSON(jsonrpc.APIVersion3)
	assert.NoError(t, err)
	evs, err := json.Marshal(evj)
	t.Logf("JSON:%s", evs)

	bs, err := codec.MarshalToBytes(&ev)
	assert.NoError(t, err)

	log.Printf("ENCODED:% x", bs)

	var ev2 eventLog
	_, err = codec.UnmarshalFromBytes(bs, &ev2)
	assert.NoError(t, err)

	evj, err = ev2.ToJSON(jsonrpc.APIVersion3)
	assert.NoError(t, err)
	evs2, err := json.Marshal(evj)
	t.Logf("JSON:%s", evs2)

	assert.Equal(t, evs, evs2)
}
