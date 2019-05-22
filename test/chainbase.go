// Code generated by go generate; DO NOT EDIT.
package test

import (
	"context"

	"github.com/icon-project/goloop/common/db"
	"github.com/icon-project/goloop/module"
)

type ChainBase struct{}

func (_r *ChainBase) Database() db.Database {
	panic("not implemented")
}

func (_r *ChainBase) Wallet() module.Wallet {
	panic("not implemented")
}

func (_r *ChainBase) NID() int {
	panic("not implemented")
}

func (_r *ChainBase) ConcurrencyLevel() int {
	panic("not implemented")
}

func (_r *ChainBase) Genesis() []byte {
	panic("not implemented")
}

func (_r *ChainBase) GetGenesisData(key []byte) ([]byte, error) {
	panic("not implemented")
}

func (_r *ChainBase) CommitVoteSetDecoder() module.CommitVoteSetDecoder {
	panic("not implemented")
}

func (_r *ChainBase) BlockManager() module.BlockManager {
	panic("not implemented")
}

func (_r *ChainBase) Consensus() module.Consensus {
	panic("not implemented")
}

func (_r *ChainBase) ServiceManager() module.ServiceManager {
	panic("not implemented")
}

func (_r *ChainBase) NetworkManager() module.NetworkManager {
	panic("not implemented")
}

func (_r *ChainBase) Regulator() module.Regulator {
	panic("not implemented")
}

func (_r *ChainBase) Init(sync bool) error {
	panic("not implemented")
}

func (_r *ChainBase) Start(sync bool) error {
	panic("not implemented")
}

func (_r *ChainBase) Stop(sync bool) error {
	panic("not implemented")
}

func (_r *ChainBase) Term(sync bool) error {
	panic("not implemented")
}

func (_r *ChainBase) State() string {
	panic("not implemented")
}

func (_r *ChainBase) Reset(sync bool) error {
	panic("not implemented")
}

func (_r *ChainBase) Verify(sync bool) error {
	panic("not implemented")
}

func (_r *ChainBase) MetricContext() context.Context {
	panic("not implemented")
}