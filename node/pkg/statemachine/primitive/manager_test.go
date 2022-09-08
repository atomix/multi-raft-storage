// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package primitive

import (
	multiraftv1 "github.com/atomix/multi-raft-storage/api/atomix/multiraft/v1"
	"github.com/atomix/multi-raft-storage/node/pkg/statemachine"
	"github.com/atomix/multi-raft-storage/node/pkg/statemachine/session"
	"github.com/atomix/runtime/sdk/pkg/logging"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func newManagerTestContext(ctrl *gomock.Controller) *managerTestContext {
	context := session.NewMockContext(ctrl)
	context.EXPECT().Time().Return(time.UnixMilli(0)).AnyTimes()
	context.EXPECT().Log().Return(logging.GetLogger()).AnyTimes()
	return &managerTestContext{
		MockContext: context,
	}
}

type managerTestContext struct {
	*session.MockContext
	index   statemachine.Index
	queryID statemachine.QueryID
}

func (c *managerTestContext) nextProposalID() statemachine.ProposalID {
	c.index++
	c.EXPECT().Index().Return(c.index).AnyTimes()
	return statemachine.ProposalID(c.index)
}

func (c *managerTestContext) nextQueryID() statemachine.QueryID {
	c.queryID++
	return c.queryID
}

func TestPrimitive(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	context := newManagerTestContext(ctrl)
	timer := statemachine.NewMockTimer(ctrl)
	scheduler := statemachine.NewMockScheduler(ctrl)
	scheduler.EXPECT().Schedule(gomock.Any(), gomock.Any()).Return(timer).AnyTimes()
	context.EXPECT().Scheduler().Return(scheduler).AnyTimes()

	registry := NewTypeRegistry()
	codec := NewMockAnyCodec(ctrl)
	codec.EXPECT().DecodeInput(gomock.Any()).Return("Hello", nil).AnyTimes()
	codec.EXPECT().EncodeOutput(gomock.Any()).Return([]byte("world!"), nil).AnyTimes()

	var primitiveCtx Context[any, any]
	primitive := NewMockAnyPrimitive(ctrl)
	primitiveType := NewType[any, any]("test", codec, func(ctx Context[any, any]) Primitive[any, any] {
		primitiveCtx = ctx
		assert.Equal(t, "test", ctx.Service())
		assert.Equal(t, "test-namespace", ctx.Namespace())
		assert.Equal(t, "test-name", ctx.Name())
		return primitive
	})
	RegisterType[any, any](registry)(primitiveType)

	manager := NewManager(context, registry)

	session1 := session.NewMockSession(ctrl)
	session1.EXPECT().ID().Return(session.ID(1)).AnyTimes()
	session1.EXPECT().Log().Return(logging.GetLogger()).AnyTimes()
	session1.EXPECT().Watch(gomock.Any()).Return(func() {}).AnyTimes()

	session2 := session.NewMockSession(ctrl)
	session2.EXPECT().ID().Return(session.ID(2)).AnyTimes()
	session2.EXPECT().Log().Return(logging.GetLogger()).AnyTimes()
	session2.EXPECT().Watch(gomock.Any()).Return(func() {}).AnyTimes()

	sessions := session.NewMockSessions(ctrl)
	sessions.EXPECT().Get(gomock.Any()).DoAndReturn(func(id session.ID) (session.Session, bool) {
		if id == 1 {
			return session1, true
		}
		if id == 2 {
			return session2, true
		}
		return nil, false
	}).AnyTimes()
	sessions.EXPECT().List().Return([]session.Session{session1, session2}).AnyTimes()
	context.EXPECT().Sessions().Return(sessions).AnyTimes()

	createPrimitive := session.NewMockCreatePrimitiveProposal(ctrl)
	createPrimitive.EXPECT().ID().Return(context.nextProposalID()).AnyTimes()
	createPrimitive.EXPECT().Session().Return(session1).AnyTimes()
	createPrimitive.EXPECT().Input().Return(&multiraftv1.CreatePrimitiveInput{
		PrimitiveSpec: multiraftv1.PrimitiveSpec{
			Service:   "test",
			Namespace: "test-namespace",
			Name:      "test-name",
		},
	}).AnyTimes()
	var primitiveID multiraftv1.PrimitiveID
	createPrimitive.EXPECT().Output(gomock.Any()).Do(func(output *multiraftv1.CreatePrimitiveOutput) {
		primitiveID = output.PrimitiveID
	})
	createPrimitive.EXPECT().Close()
	manager.CreatePrimitive(createPrimitive)

	unusedProposal := session.NewMockPrimitiveProposal(ctrl)
	unusedProposalID := context.nextProposalID()
	unusedProposal.EXPECT().ID().Return(unusedProposalID).AnyTimes()
	unusedProposal.EXPECT().Session().Return(session1).AnyTimes()
	unusedProposal.EXPECT().Input().Return(&multiraftv1.PrimitiveProposalInput{
		PrimitiveID: 0,
	}).AnyTimes()

	proposal := session.NewMockPrimitiveProposal(ctrl)
	proposalID := context.nextProposalID()
	proposal.EXPECT().ID().Return(proposalID).AnyTimes()
	proposal.EXPECT().Session().Return(session1).AnyTimes()
	proposal.EXPECT().Input().Return(&multiraftv1.PrimitiveProposalInput{
		PrimitiveID: primitiveID,
		Payload:     []byte("Hello"),
	}).AnyTimes()
	proposal.EXPECT().Output(gomock.Any())
	proposal.EXPECT().Close()
	proposals := session.NewMockProposals(ctrl)
	proposals.EXPECT().Get(gomock.Any()).DoAndReturn(func(id statemachine.ProposalID) (session.PrimitiveProposal, bool) {
		if id == proposalID {
			return proposal, true
		}
		if id == unusedProposalID {
			return unusedProposal, true
		}
		return nil, false
	}).AnyTimes()
	proposals.EXPECT().List().Return([]session.PrimitiveProposal{proposal, unusedProposal}).AnyTimes()
	context.EXPECT().Proposals().Return(proposals).AnyTimes()
	session1.EXPECT().Proposals().Return(proposals).AnyTimes()
	primitive.EXPECT().Propose(gomock.Any()).Do(func(proposal Proposal[any, any]) {
		assert.Equal(t, ID(primitiveID), primitiveCtx.ID())
		assert.Len(t, primitiveCtx.Sessions().List(), 1)
		session, ok := primitiveCtx.Sessions().Get(1)
		assert.True(t, ok)
		assert.Equal(t, SessionID(1), session.ID())
		assert.Len(t, primitiveCtx.Proposals().List(), 1)
		assert.Equal(t, proposal.ID(), primitiveCtx.Proposals().List()[0].ID())
		p, ok := primitiveCtx.Proposals().Get(proposal.ID())
		assert.True(t, ok)
		assert.Equal(t, proposal.ID(), p.ID())
		assert.Len(t, session.Proposals().List(), 1)
		assert.Equal(t, proposal.ID(), session.Proposals().List()[0].ID())
		p, ok = session.Proposals().Get(proposal.ID())
		assert.True(t, ok)
		assert.Equal(t, proposal.ID(), p.ID())
		assert.Equal(t, SessionID(1), proposal.Session().ID())
		assert.Len(t, proposal.Session().Proposals().List(), 1)
		assert.Equal(t, proposal.ID(), proposal.Session().Proposals().List()[0].ID())
		p, ok = proposal.Session().Proposals().Get(proposal.ID())
		assert.True(t, ok)
		assert.Equal(t, proposal.ID(), p.ID())
		proposal.Output("world!")
		proposal.Close()
	})
	manager.Propose(proposal)

	query := session.NewMockPrimitiveQuery(ctrl)
	query.EXPECT().ID().Return(context.nextQueryID()).AnyTimes()
	query.EXPECT().Session().Return(session1).AnyTimes()
	query.EXPECT().Input().Return(&multiraftv1.PrimitiveQueryInput{
		PrimitiveID: primitiveID,
		Payload:     []byte("Hello"),
	}).AnyTimes()
	query.EXPECT().Output(gomock.Any())
	query.EXPECT().Close()
	primitive.EXPECT().Query(gomock.Any()).Do(func(query Query[any, any]) {
		assert.Equal(t, ID(primitiveID), primitiveCtx.ID())
		assert.Len(t, primitiveCtx.Sessions().List(), 1)
		session, ok := primitiveCtx.Sessions().Get(1)
		assert.True(t, ok)
		assert.Equal(t, SessionID(1), session.ID())
		query.Output("world!")
		query.Close()
	})
	manager.Query(query)

	closePrimitive := session.NewMockClosePrimitiveProposal(ctrl)
	closePrimitive.EXPECT().ID().Return(context.nextProposalID()).AnyTimes()
	closePrimitive.EXPECT().Session().Return(session1).AnyTimes()
	closePrimitive.EXPECT().Input().Return(&multiraftv1.ClosePrimitiveInput{
		PrimitiveID: primitiveID,
	}).AnyTimes()
	closePrimitive.EXPECT().Output(gomock.Any())
	closePrimitive.EXPECT().Close()
	manager.ClosePrimitive(closePrimitive)
}