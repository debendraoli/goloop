package imports

import (
	"bytes"
	"encoding/json"

	"github.com/icon-project/goloop/common/errors"
	"github.com/icon-project/goloop/common/legacy"
	"github.com/icon-project/goloop/module"
	"github.com/icon-project/goloop/service"
	"github.com/icon-project/goloop/service/eeproxy"
)

type ImportCallback interface {
	OnError(err error)
	OnEnd()
}

type managerForImport struct {
	module.ServiceManager
	bdb        *legacy.LoopChainDB
	lastHeight int64
	cb         ImportCallback
}

func NewServiceManagerForImport(chain module.Chain, nm module.NetworkManager,
	eem eeproxy.Manager, contractDir string, lcDBDir string,
	height int64, cb ImportCallback,
) (module.ServiceManager, module.Timestamper, error) {
	manager, err := service.NewManager(chain, nm, eem, contractDir)
	if err != nil {
		return nil, nil, err
	}
	bdb, err := legacy.OpenDatabase(lcDBDir, lcDBDir)
	if err != nil {
		return nil, nil, err
	}
	blk, err := bdb.GetLastBlock()
	if err != nil || blk.Height() < height {
		return nil, nil, err
	}
	m := &managerForImport{
		ServiceManager: manager,
		bdb:            bdb,
		lastHeight:     height,
		cb:             cb,
	}
	return m, m, nil
}

func (m *managerForImport) GetVoteTimestamp(h, ts int64) int64 {
	if h >= m.lastHeight {
		return ts
	}
	blk, err := m.bdb.GetBlockByHeight(int(h + 1))
	if err != nil {
		m.cb.OnError(err)
		return ts
	}
	return blk.Timestamp()
}

func (m *managerForImport) GetBlockTimestamp(h, ts int64) int64 {
	if h == 1 {
		blk, err := m.bdb.GetBlockByHeight(int(h))
		if err != nil {
			m.cb.OnError(err)
			return ts
		}
		ts = blk.Timestamp()
	}
	return ts
}

func unwrap(tr module.Transition) module.Transition {
	return tr.(*transitionForImport).Transition
}

type blockInfo struct {
	height    int64
	timestamp int64
}

func (bi blockInfo) Height() int64 {
	return bi.height
}

func (bi blockInfo) Timestamp() int64 {
	return bi.timestamp
}

func (m *managerForImport) ProposeTransition(parent module.Transition, bi module.BlockInfo) (module.Transition, error) {
	blk, err := m.bdb.GetBlockByHeight(int(bi.Height()))
	if err != nil {
		m.cb.OnError(err)
		return nil, err
	}
	if bi.Height() == 1 {
		bi = &blockInfo{1, blk.Timestamp()}
	}
	txl := blk.NormalTransactions()
	var txs []module.Transaction
	for it := txl.Iterator(); it.Has(); it.Next() {
		tx, _, _ := it.Get()
		txs = append(txs, tx)
	}
	txl2 := m.ServiceManager.TransactionListFromSlice(txs, module.BlockVersion2)
	otr, err := m.ServiceManager.CreateTransition(unwrap(parent), txl2, bi)
	if err != nil {
		return nil, err
	}
	return &transitionForImport{
		Transition: otr,
		m:          m,
		bi:         bi,
	}, nil
}

func (m *managerForImport) CreateInitialTransition(result []byte, nextValidators module.ValidatorList) (module.Transition, error) {
	otr, err := m.ServiceManager.CreateInitialTransition(result, nextValidators)
	if err != nil {
		return nil, err
	}
	return &transitionForImport{
		Transition: otr,
		m:          m,
	}, nil
}

func (m *managerForImport) CreateTransition(parent module.Transition, txs module.TransactionList, bi module.BlockInfo) (module.Transition, error) {
	otr, err := m.ServiceManager.CreateTransition(unwrap(parent), txs, bi)
	if err != nil {
		return nil, err
	}
	return &transitionForImport{
		Transition: otr,
		m:          m,
		bi:         bi,
	}, nil
}

func (m *managerForImport) GetPatches(parent module.Transition) module.TransactionList {
	return m.ServiceManager.GetPatches(unwrap(parent))
}

func (m *managerForImport) PatchTransition(transition module.Transition, patches module.TransactionList) module.Transition {
	otr := m.ServiceManager.PatchTransition(unwrap(transition), patches)
	if otr == nil {
		return nil
	}
	return &transitionForImport{
		Transition: otr,
		m:          m,
		bi:         transition.(*transitionForImport).bi,
	}
}

func (m *managerForImport) Finalize(transition module.Transition, opt int) error {
	if opt&module.FinalizeNormalTransaction != 0 {
		h := transition.(*transitionForImport).bi.Height()
		if h >= m.lastHeight {
			cb := m.cb
			go func() {
				cb.OnEnd()
			}()
		}
	}
	return m.finalize(transition, opt)
}

func (m *managerForImport) finalize(transition module.Transition, opt int) error {
	return m.ServiceManager.Finalize(unwrap(transition), opt)
}

type transitionForImport struct {
	module.Transition
	m        *managerForImport
	bi       module.BlockInfo
	cb       module.TransitionCallback
	canceler func() bool
}

func (t *transitionForImport) OnValidate(tr module.Transition, e error) {
	if t.bi.Height() == 0 {
		t.cb.OnValidate(t, e)
		return
	}
	if e != nil {
		t.cb.OnValidate(t, e)
		return
	}
	blk, err := t.m.bdb.GetBlockByHeight(int(t.bi.Height()))
	if err != nil {
		t.m.cb.OnError(err)
		t.cb.OnValidate(t, err)
		t.canceler()
		return
	}
	txl := blk.NormalTransactions()
	var txs []module.Transaction
	for it := txl.Iterator(); it.Has(); it.Next() {
		tx, _, _ := it.Get()
		txs = append(txs, tx)
	}
	txl2 := t.m.ServiceManager.TransactionListFromSlice(txs, module.BlockVersion2)
	if txl2.Equal(t.NormalTransactions()) {
		t.cb.OnValidate(t, nil)
	} else {
		t.cb.OnValidate(t, errors.New("transaction list is different"))
		t.canceler()
	}
}

func (t *transitionForImport) OnExecute(tr module.Transition, e error) {
	if t.bi.Height() == 0 {
		t.cb.OnExecute(t, e)
		return
	}
	if e != nil {
		t.cb.OnExecute(t, e)
		return
	}
	blk, err := t.m.bdb.GetBlockByHeight(int(t.bi.Height()))
	if err != nil {
		t.m.cb.OnError(err)
		t.cb.OnExecute(t, err)
		t.canceler()
		return
	}
	txl := blk.NormalTransactions()
	t.m.finalize(t, module.FinalizeNormalTransaction|module.FinalizePatchTransaction|module.FinalizeResult)
	rl, err := t.m.ReceiptListFromResult(tr.Result(), module.TransactionGroupNormal)
	if err != nil {
		t.m.cb.OnError(err)
		t.cb.OnExecute(t, err)
		t.canceler()
		return
	}
	rit := rl.Iterator()
	for i := txl.Iterator(); i.Has(); i.Next() {
		tx, _, err := i.Get()
		if err != nil {
			t.m.cb.OnError(err)
			t.cb.OnExecute(t, err)
			t.canceler()
			return
		}
		rct, err := t.m.bdb.GetReceiptByTransaction(tx.ID())
		if err != nil {
			t.m.cb.OnError(err)
			t.cb.OnExecute(t, err)
			t.canceler()
			return
		}
		nrct, err := rit.Get()
		if err != nil {
			t.m.cb.OnError(err)
			t.cb.OnExecute(t, err)
			t.canceler()
			return
		}
		rjsn, _ := rct.ToJSON(3)
		mrjsn := rjsn.(map[string]interface{})
		delete(mrjsn, "failure")
		rjbs, _ := json.Marshal(mrjsn)

		nrjsn, _ := nrct.ToJSON(3)
		mnrjsn := nrjsn.(map[string]interface{})
		delete(mnrjsn, "failure")
		nrjbs, _ := json.Marshal(mnrjsn)
		if !bytes.Equal(rjbs, nrjbs) {
			err = errors.Errorf("cannot agree with receipt lc:%s gc:%s tx:%x", rjbs, nrjbs, tx.ID())
			t.m.cb.OnError(err)
			t.cb.OnExecute(t, err)
			t.canceler()
			return
		}
		rit.Next()
	}
	t.cb.OnExecute(t, nil)
}

func (t *transitionForImport) Execute(cb module.TransitionCallback) (canceler func() bool, err error) {
	t.cb = cb
	c, e := t.Transition.Execute(t)
	t.canceler = c
	return c, e
}
