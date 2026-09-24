package org

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/exoin/ciftpay/internal/fiscal/mock"
	"github.com/exoin/ciftpay/internal/platform/db/dbtest"
)

func TestAccountantInvitationLifecycle(t *testing.T) {
	d := dbtest.Open(t)
	ctx := context.Background()
	s := newTestService(t, d, NewFiscalPINChecker(mock.New(mock.FailNone), time.Second))

	// 1. Merchant creates an organization
	merchant := createTestUser(t, s, "254711111111")
	org, err := s.CreateOrg(ctx, merchant.ID, CreateOrgInput{Name: "Acme Groceries Ltd", KRAPin: "P051234567A"})
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// 2. Accountant user exists
	accountantPhone := "0722000111"
	accountant := createTestUser(t, s, "254722000111")

	// Other random user
	otherUser := createTestUser(t, s, "254733333333")

	// 3. Merchant invites the accountant
	invite, err := s.InviteMember(ctx, org.ID, merchant.ID, InviteInput{
		Phone: accountantPhone,
		Role:  RoleAccountant,
	})
	if err != nil {
		t.Fatalf("InviteMember: %v", err)
	}
	if invite.Status != "pending" {
		t.Fatalf("expected invite status 'pending', got %q", invite.Status)
	}

	// 4. Listing invites for the org shows the pending invite
	orgInvites, err := s.ListInvites(ctx, org.ID)
	if err != nil {
		t.Fatalf("ListInvites: %v", err)
	}
	if len(orgInvites) != 1 || orgInvites[0].ID != invite.ID {
		t.Fatalf("expected 1 invite with id %s, got %d", invite.ID, len(orgInvites))
	}

	// 5. Accountant lists pending invites
	pendingInvites, err := s.ListPendingInvitesForUser(ctx, accountant.ID)
	if err != nil {
		t.Fatalf("ListPendingInvitesForUser: %v", err)
	}
	if len(pendingInvites) != 1 {
		t.Fatalf("expected 1 pending invite for accountant, got %d", len(pendingInvites))
	}
	if pendingInvites[0].OrgName != "Acme Groceries Ltd" {
		t.Errorf("expected org name 'Acme Groceries Ltd', got %q", pendingInvites[0].OrgName)
	}
	if pendingInvites[0].KRAPin != "P051234567A" {
		t.Errorf("expected KRA PIN 'P051234567A', got %q", pendingInvites[0].KRAPin)
	}

	// 6. Other user trying to accept the invite is rejected with ErrForbidden
	err = s.AcceptInvite(ctx, otherUser.ID, invite.ID)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for foreign user, got: %v", err)
	}

	// 7. Accountant accepts the invite
	err = s.AcceptInvite(ctx, accountant.ID, invite.ID)
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}

	// 8. Pending invites should now be empty
	pendingAfterAccept, err := s.ListPendingInvitesForUser(ctx, accountant.ID)
	if err != nil {
		t.Fatalf("ListPendingInvitesForUser after accept: %v", err)
	}
	if len(pendingAfterAccept) != 0 {
		t.Fatalf("expected 0 pending invites after accept, got %d", len(pendingAfterAccept))
	}

	// 9. Accountant should now see the client in ListAccountantClients
	clients, err := s.ListAccountantClients(ctx, accountant.ID)
	if err != nil {
		t.Fatalf("ListAccountantClients: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client org, got %d", len(clients))
	}
	if clients[0].OrgID != org.ID {
		t.Errorf("expected OrgID %s, got %s", org.ID, clients[0].OrgID)
	}
	if clients[0].Name != "Acme Groceries Ltd" {
		t.Errorf("expected client name 'Acme Groceries Ltd', got %q", clients[0].Name)
	}
	if clients[0].KRAPin != "P051234567A" {
		t.Errorf("expected client KRA PIN 'P051234567A', got %q", clients[0].KRAPin)
	}
	if clients[0].Role != RoleAccountant {
		t.Errorf("expected role 'accountant', got %q", clients[0].Role)
	}

	// 10. Test rejection on a second invite
	invite2, err := s.InviteMember(ctx, org.ID, merchant.ID, InviteInput{
		Phone: "0733333333",
		Role:  RoleAccountant,
	})
	if err != nil {
		t.Fatalf("InviteMember 2: %v", err)
	}

	err = s.RejectInvite(ctx, otherUser.ID, invite2.ID)
	if err != nil {
		t.Fatalf("RejectInvite: %v", err)
	}

	// Rejection should prevent subsequent acceptance
	err = s.AcceptInvite(ctx, otherUser.ID, invite2.ID)
	if err == nil {
		t.Fatal("expected error accepting already rejected invite, got nil")
	}
}


func TestCreateAccountantOrg(t *testing.T) {
	d := dbtest.Open(t)
	ctx := context.Background()
	s := newTestService(t, d, NewFiscalPINChecker(mock.New(mock.FailNone), time.Second))

	accountant := createTestUser(t, s, "254700999888")

	// Create accountant practice without KRA PIN
	org, err := s.CreateOrg(ctx, accountant.ID, CreateOrgInput{
		Name:    "Mwangi & Partners Tax Advisory",
		Role:    RoleAccountant,
		Profile: "accountant",
	})
	if err != nil {
		t.Fatalf("CreateOrg accountant: %v", err)
	}

	if org.Name != "Mwangi & Partners Tax Advisory" {
		t.Errorf("expected name Mwangi & Partners Tax Advisory, got %q", org.Name)
	}
	if org.Role != RoleAccountant {
		t.Errorf("expected org role accountant, got %q", org.Role)
	}

	memberships, err := s.ListMemberships(ctx, accountant.ID)
	if err != nil {
		t.Fatalf("ListMemberships: %v", err)
	}
	if len(memberships) != 1 {
		t.Fatalf("expected 1 membership, got %d", len(memberships))
	}
	if memberships[0].Role != RoleAccountant {
		t.Errorf("expected membership role accountant, got %q", memberships[0].Role)
	}
}
