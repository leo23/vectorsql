// Copyright 2020 The VectorSQL Authors.
//
// Code is licensed under Apache License, Version 2.0.

package processors

import (
	"context"
	"time"

	"base/counter"
)

type (
	NextFunc     func(interface{})
	DoneFunc     func()
	EventHandler interface{}

	BaseProcessor struct {
		in          *InPort
		out         *OutPort
		name        string
		ctx         context.Context
		counter     *counter.Counter
		pauseChan   chan struct{}
		resumeChan  chan struct{}
		nextHandler NextFunc
		doneHandler DoneFunc
	}
)

func NewBaseProcessor(name string) BaseProcessor {
	return BaseProcessor{
		in:         NewInPort(name),
		out:        NewOutPort(name),
		ctx:        context.Background(),
		name:       name,
		counter:    counter.NewCounter(),
		pauseChan:  make(chan struct{}),
		resumeChan: make(chan struct{}),
	}
}

func (p *BaseProcessor) Name() string {
	return p.name
}

func (p *BaseProcessor) In() *InPort {
	return p.in
}

func (p *BaseProcessor) Out() *OutPort {
	return p.out
}

func (p *BaseProcessor) Metric() counter.Metric {
	return p.counter.Metric()
}

func (p *BaseProcessor) To(receivers ...IProcessor) {
	for _, receiver := range receivers {
		p.out.To(receiver.In())
	}
}

func (p *BaseProcessor) From(senders ...IProcessor) {
	for _, sender := range senders {
		source := sender.Out()
		p.in.From(source)
	}
}

func (p *BaseProcessor) Execute() {
	// Nothing.
}

func (p *BaseProcessor) Pause() {
	p.pauseChan <- struct{}{}
}

func (p *BaseProcessor) Resume() {
	p.resumeChan <- struct{}{}
}

func (p *BaseProcessor) SetContext(ctx context.Context) {
	p.ctx = ctx
}

func (p *BaseProcessor) AddMessage(n int64) {
	p.counter.AddMessage(n)
}

func (p *BaseProcessor) AddDuration(n time.Duration) {
	p.counter.AddDuration(n)
}

func (p *BaseProcessor) Subscribe(eventHandlers ...EventHandler) {
	in := p.In()
	out := p.Out()
	ctx := p.ctx
	counter := p.counter

	for _, handler := range eventHandlers {
		switch handler := handler.(type) {
		case func():
			p.doneHandler = handler
		case func(interface{}):
			p.nextHandler = handler
		}
	}

	start := time.Now()
	defer func() {
		out.Close()
		close(p.pauseChan)
		close(p.resumeChan)
	}()

	for {
	Loop:
		select {
		case <-p.pauseChan:
			for range p.resumeChan {
				goto Loop
			}
		case <-ctx.Done():
			if p.nextHandler != nil {
				p.nextHandler(ctx.Err())
			}
			return
		case x, ok := <-in.Recv():
			if !ok {
				if p.doneHandler != nil {
					p.doneHandler()
				}
				return
			}
			counter.AddLatency(time.Since(start))
			if p.nextHandler != nil {
				p.nextHandler(x)
			}
			start = time.Now()
		}
	}
}
