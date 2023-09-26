package main

import (
	"context"
	"sync"

	"github.com/cockroachdb/errors"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	ipld "github.com/ipfs/go-ipld-format"
)

var _ blockstore.Blockstore = &multiBlockstore{}

type multiBlockstore struct {
	bss []blockstore.Blockstore
	mu  sync.RWMutex
}

var ErrNotImplemented = errors.New("not implemented")

func (m *multiBlockstore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	return ErrNotImplemented
}

func (m *multiBlockstore) AddBlockstore(bs blockstore.Blockstore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bss = append(m.bss, bs)
}

func (m *multiBlockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, bs := range m.bss {
		has, err := bs.Has(ctx, c)
		if err != nil {
			return false, errors.WithStack(err)
		}
		if has {
			return true, nil
		}
	}
	return false, nil
}

func (m *multiBlockstore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, bs := range m.bss {
		block, err := bs.Get(ctx, c)
		if errors.Is(err, ipld.ErrNotFound{}) {
			continue
		}
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if block != nil {
			return block, nil
		}
	}
	return nil, ipld.ErrNotFound{Cid: c}
}

func (m *multiBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, bs := range m.bss {
		size, err := bs.GetSize(ctx, c)
		if errors.Is(err, ipld.ErrNotFound{}) {
			continue
		}
		if err != nil {
			return 0, errors.WithStack(err)
		}
		if size > 0 {
			return size, nil
		}
	}
	return 0, ipld.ErrNotFound{Cid: c}
}

func (m *multiBlockstore) Put(ctx context.Context, block blocks.Block) error {
	return ErrNotImplemented
}

func (m *multiBlockstore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	return ErrNotImplemented
}

func (m *multiBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return nil, ErrNotImplemented
}

func (m *multiBlockstore) HashOnRead(enabled bool) {
}
