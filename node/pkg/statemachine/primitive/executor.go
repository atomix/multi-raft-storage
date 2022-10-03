// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package primitive

import (
	"github.com/atomix/multi-raft-storage/node/pkg/statemachine"
	"github.com/atomix/multi-raft-storage/node/pkg/statemachine/session"
	"github.com/atomix/runtime/sdk/pkg/logging"
	"github.com/atomix/runtime/sdk/pkg/stringer"
	"github.com/gogo/protobuf/proto"
	"time"
)

type Executor[T statemachine.Call[U, I, O], U statemachine.CallID, I, O any] interface {
	Execute(T)
}

type Proposer[I1, O1, I2, O2 any] interface {
	Executor[Proposal[I1, O1], statemachine.ProposalID, I1, O1]
	Proposals() Proposals[I2, O2]
}

func NewProposer[I1, O1, I2, O2 proto.Message](ctx Context[I1, O1]) *ProposerBuilder[I1, O1, I2, O2] {
	return &ProposerBuilder[I1, O1, I2, O2]{
		ctx: ctx,
	}
}

type ProposerBuilder[I1, O1, I2, O2 proto.Message] struct {
	ctx     Context[I1, O1]
	name    string
	decoder func(I1) (I2, bool)
	encoder func(O2) O1
}

func (b *ProposerBuilder[I1, O1, I2, O2]) Name(name string) *ProposerBuilder[I1, O1, I2, O2] {
	b.name = name
	return b
}

func (b *ProposerBuilder[I1, O1, I2, O2]) Decoder(f func(I1) (I2, bool)) *ProposerBuilder[I1, O1, I2, O2] {
	b.decoder = f
	return b
}

func (b *ProposerBuilder[I1, O1, I2, O2]) Encoder(f func(O2) O1) *ProposerBuilder[I1, O1, I2, O2] {
	b.encoder = f
	return b
}

func (b *ProposerBuilder[I1, O1, I2, O2]) Build(f func(Proposal[I2, O2])) Proposer[I1, O1, I2, O2] {
	return &transcodingProposer[I1, O1, I2, O2]{
		ctx:     b.ctx,
		decoder: b.decoder,
		encoder: b.encoder,
		name:    b.name,
		f:       f,
	}
}

var _ ExecutorBuilder[
	Proposal[proto.Message, proto.Message],
	statemachine.ProposalID,
	proto.Message,
	proto.Message,
	Proposer[proto.Message, proto.Message, proto.Message, proto.Message]] = (*ProposerBuilder[proto.Message, proto.Message, proto.Message, proto.Message])(nil)

type ExecutorBuilder[
	T session.Call[U, I, O],
	U statemachine.CallID,
	I proto.Message,
	O proto.Message,
	E Executor[T, U, I, O]] interface {
	Build(f func(T)) E
}

type Querier[I1, O1, I2, O2 any] interface {
	Executor[Query[I1, O1], statemachine.QueryID, I1, O1]
}

func NewQuerier[I1, O1, I2, O2 proto.Message](ctx Context[I1, O1]) *QuerierBuilder[I1, O1, I2, O2] {
	return &QuerierBuilder[I1, O1, I2, O2]{
		ctx: ctx,
	}
}

type QuerierBuilder[I1, O1, I2, O2 proto.Message] struct {
	ctx     Context[I1, O1]
	name    string
	decoder func(I1) (I2, bool)
	encoder func(O2) O1
}

func (b *QuerierBuilder[I1, O1, I2, O2]) Name(name string) *QuerierBuilder[I1, O1, I2, O2] {
	b.name = name
	return b
}

func (b *QuerierBuilder[I1, O1, I2, O2]) Decoder(f func(I1) (I2, bool)) *QuerierBuilder[I1, O1, I2, O2] {
	b.decoder = f
	return b
}

func (b *QuerierBuilder[I1, O1, I2, O2]) Encoder(f func(O2) O1) *QuerierBuilder[I1, O1, I2, O2] {
	b.encoder = f
	return b
}

func (b *QuerierBuilder[I1, O1, I2, O2]) Build(f func(Query[I2, O2])) Querier[I1, O1, I2, O2] {
	return &transcodingQuerier[I1, O1, I2, O2]{
		ctx:     b.ctx,
		decoder: b.decoder,
		encoder: b.encoder,
		name:    b.name,
		f:       f,
	}
}

var _ ExecutorBuilder[
	Query[proto.Message, proto.Message],
	statemachine.QueryID,
	proto.Message,
	proto.Message,
	Querier[proto.Message, proto.Message, proto.Message, proto.Message]] = (*QuerierBuilder[proto.Message, proto.Message, proto.Message, proto.Message])(nil)

type transcodingProposer[I1, O1, I2, O2 proto.Message] struct {
	ctx     Context[I1, O1]
	decoder func(I1) (I2, bool)
	encoder func(O2) O1
	name    string
	f       func(Proposal[I2, O2])
}

func (p *transcodingProposer[I1, O1, I2, O2]) Proposals() Proposals[I2, O2] {
	return newTranscodingProposals[I1, O1, I2, O2](p.ctx.Proposals(), p.decoder, p.encoder)
}

func (p *transcodingProposer[I1, O1, I2, O2]) Execute(parent Proposal[I1, O1]) {
	input, ok := p.decoder(parent.Input())
	if !ok {
		return
	}
	proposal := newTranscodingProposal[I1, O1, I2, O2](parent, input, p.decoder, p.encoder, parent.Log().WithFields(logging.String("Method", p.name)))
	proposal.Log().Debugw("Applying proposal", logging.Stringer("Input", stringer.Truncate(proposal.Input(), truncLen)))
	p.f(proposal)
}

type transcodingQuerier[I1, O1, I2, O2 proto.Message] struct {
	ctx     Context[I1, O1]
	decoder func(I1) (I2, bool)
	encoder func(O2) O1
	name    string
	f       func(Query[I2, O2])
}

func (q *transcodingQuerier[I1, O1, I2, O2]) Execute(parent Query[I1, O1]) {
	input, ok := q.decoder(parent.Input())
	if !ok {
		return
	}
	query := newTranscodingQuery[I1, O1, I2, O2](parent, input, q.decoder, q.encoder, parent.Log().WithFields(logging.String("Method", q.name)))
	query.Log().Debugw("Applying query", logging.Stringer("Input", stringer.Truncate(query.Input(), truncLen)))
	q.f(query)
}

func newTranscodingProposals[I1, O1, I2, O2 any](parent Proposals[I1, O1], decoder func(I1) (I2, bool), encoder func(O2) O1) Proposals[I2, O2] {
	return &transcodingProposals[I1, O1, I2, O2]{
		parent:  parent,
		decoder: decoder,
		encoder: encoder,
	}
}

type transcodingProposals[I1, O1, I2, O2 any] struct {
	parent  Proposals[I1, O1]
	decoder func(I1) (I2, bool)
	encoder func(O2) O1
}

func (p *transcodingProposals[I1, O1, I2, O2]) Get(id ProposalID) (Proposal[I2, O2], bool) {
	parent, ok := p.parent.Get(id)
	if !ok {
		return nil, false
	}
	if input, ok := p.decoder(parent.Input()); ok {
		return newTranscodingProposal[I1, O1, I2, O2](parent, input, p.decoder, p.encoder, parent.Log()), true
	}
	return nil, false
}

func (p *transcodingProposals[I1, O1, I2, O2]) List() []Proposal[I2, O2] {
	parents := p.parent.List()
	proposals := make([]Proposal[I2, O2], 0, len(parents))
	for _, parent := range parents {
		if input, ok := p.decoder(parent.Input()); ok {
			proposal := newTranscodingProposal[I1, O1, I2, O2](parent, input, p.decoder, p.encoder, parent.Log())
			proposals = append(proposals, proposal)
		}
	}
	return proposals
}

func newTranscodingCall[T statemachine.CallID, I1, O1, I2, O2 any](
	parent session.Call[T, I1, O1],
	input I2,
	decoder func(I1) (I2, bool),
	encoder func(O2) O1,
	log logging.Logger) session.Call[T, I2, O2] {
	return &transcodingCall[T, I1, O1, I2, O2]{
		parent:  parent,
		input:   input,
		decoder: decoder,
		encoder: encoder,
		log:     log,
	}
}

type transcodingCall[T statemachine.CallID, I1, O1, I2, O2 any] struct {
	parent  session.Call[T, I1, O1]
	input   I2
	decoder func(I1) (I2, bool)
	encoder func(O2) O1
	log     logging.Logger
}

func (p *transcodingCall[T, I1, O1, I2, O2]) ID() T {
	return p.parent.ID()
}

func (p *transcodingCall[T, I1, O1, I2, O2]) Log() logging.Logger {
	return p.log
}

func (p *transcodingCall[T, I1, O1, I2, O2]) Time() time.Time {
	return p.parent.Time()
}

func (p *transcodingCall[T, I1, O1, I2, O2]) Session() session.Session {
	return p.parent.Session()
}

func (p *transcodingCall[T, I1, O1, I2, O2]) State() session.CallState {
	return p.parent.State()
}

func (p *transcodingCall[T, I1, O1, I2, O2]) Watch(watcher func(state session.CallState)) session.CancelFunc {
	return p.parent.Watch(watcher)
}

func (p *transcodingCall[T, I1, O1, I2, O2]) Input() I2 {
	return p.input
}

func (p *transcodingCall[T, I1, O1, I2, O2]) Output(output O2) {
	p.parent.Output(p.encoder(output))
}

func (p *transcodingCall[T, I1, O1, I2, O2]) Error(err error) {
	p.parent.Error(err)
}

func (p *transcodingCall[T, I1, O1, I2, O2]) Cancel() {
	p.parent.Cancel()
}

func (p *transcodingCall[T, I1, O1, I2, O2]) Close() {
	p.parent.Close()
}

func newTranscodingProposal[I1, O1, I2, O2 any](
	parent Proposal[I1, O1],
	input I2,
	decoder func(I1) (I2, bool),
	encoder func(O2) O1,
	log logging.Logger) Proposal[I2, O2] {
	return newTranscodingCall[statemachine.ProposalID, I1, O1, I2, O2](parent, input, decoder, encoder, log)
}

func newTranscodingQuery[I1, O1, I2, O2 any](
	parent Query[I1, O1],
	input I2,
	decoder func(I1) (I2, bool),
	encoder func(O2) O1,
	log logging.Logger) Query[I2, O2] {
	return newTranscodingCall[statemachine.QueryID, I1, O1, I2, O2](parent, input, decoder, encoder, log)
}