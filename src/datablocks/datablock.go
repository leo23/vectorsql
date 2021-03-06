// Copyright 2020 The VectorSQL Authors.
//
// Code is licensed under Apache License, Version 2.0.

package datablocks

import (
	"columns"
	"datavalues"

	"base/errors"
)

type DataBlock struct {
	info      *DataBlockInfo
	seqs      []*datavalues.Value
	values    []*DataBlockValue
	immutable bool
	valuesmap map[string]*DataBlockValue
}

func NewDataBlock(cols []columns.Column) *DataBlock {
	var values []*DataBlockValue
	valuesmap := make(map[string]*DataBlockValue)

	for _, col := range cols {
		cv := NewDataBlockValue(col)
		valuesmap[col.Name] = cv
		values = append(values, cv)
	}
	return &DataBlock{
		info:      &DataBlockInfo{},
		values:    values,
		valuesmap: valuesmap,
	}
}

func (block *DataBlock) setSeqs(seqs []*datavalues.Value) {
	block.seqs = seqs
	block.immutable = true
}

func (block *DataBlock) Info() *DataBlockInfo {
	return block.info
}

func (block *DataBlock) NumRows() int {
	if block.seqs != nil {
		return len(block.seqs)
	} else {
		return block.values[0].NumRows()
	}
}

func (block *DataBlock) NumColumns() int {
	return len(block.values)
}

func (block *DataBlock) Columns() []columns.Column {
	var cols []columns.Column

	for _, cv := range block.values {
		cols = append(cols, cv.column)
	}
	return cols
}

func (block *DataBlock) Iterator(name string) (*DataBlockIterator, error) {
	cv, ok := block.valuesmap[name]
	if !ok {
		return nil, errors.Errorf("Can't find column:%v", name)
	}
	return newDataBlockIterator(block.seqs, cv), nil
}

func (block *DataBlock) Iterators() []*DataBlockIterator {
	var iterators []*DataBlockIterator

	for _, cv := range block.values {
		iter := newDataBlockIterator(block.seqs, cv)
		iterators = append(iterators, iter)
	}
	return iterators
}

func (block *DataBlock) Write(batcher *BatchWriter) error {
	if block.immutable {
		return errors.New("Block is immutable")
	}

	cols := batcher.values
	for _, col := range cols {
		if _, ok := block.valuesmap[col.column.Name]; !ok {
			return errors.Errorf("Can't find column:%v", col)
		}
	}

	for _, col := range cols {
		cv := block.valuesmap[col.column.Name]
		cv.values = append(cv.values, col.values...)
	}
	return nil
}

func (block *DataBlock) Split(chunksize int) []*DataBlock {
	cols := block.Columns()
	nums := block.NumRows()
	chunks := (nums / chunksize) + 1
	blocks := make([]*DataBlock, chunks)
	for i := range blocks {
		blocks[i] = NewDataBlock(cols)
	}

	for i := range cols {
		it := newDataBlockIterator(block.seqs, block.values[i])
		for j := 0; j < len(blocks); j++ {
			begin := j * chunksize
			end := (j + 1) * chunksize
			if end > nums {
				end = nums
			}
			blocks[j].values[i].values = make([]*datavalues.Value, (end - begin))
			for k := begin; k < end; k++ {
				it.Next()
				blocks[j].values[i].values[k-begin] = it.Value()
			}
		}
	}
	return blocks
}

func (block *DataBlock) SplitAsync(chunksize int) <-chan *DataBlock {
	cols := block.Columns()
	nums := block.NumRows()
	chunks := (nums / chunksize) + 1
	blocks := make([]*DataBlock, chunks)
	iters := block.Iterators()
	for i := range blocks {
		blocks[i] = NewDataBlock(cols)
	}

	ch := make(chan *DataBlock, chunks)
	go func() {
		defer close(ch)
		for j := 0; j < len(blocks); j++ {
			begin := j * chunksize
			end := (j + 1) * chunksize
			if end > nums {
				end = nums
			}
			for i := range cols {
				blocks[j].values[i].values = make([]*datavalues.Value, (end - begin))
				it := iters[i]
				for k := begin; k < end; k++ {
					it.Next()
					blocks[j].values[i].values[k-begin] = it.Value()
				}
			}
			ch <- blocks[j]
		}
	}()
	return ch
}
