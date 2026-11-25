package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/access"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/admin"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

const (
	validLeaderUUID1 = "11111111-1111-1111-1111-111111111111"
	validLeaderUUID2 = "22222222-2222-2222-2222-222222222222"
	upperLeaderUUID  = "ABCDEF12-3456-7890-ABCD-EF1234567890"
)

func TestGroupLeaderServiceEdgeCases(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("create group requires platform admin", func(t *testing.T) {
		svc := newService(&mockRepository{})
		svc.Access = &mockAccess{err: access.ErrForbidden}
		_, err := svc.CreateGroup(ctx, makePrincipal("user-2", false), api.CreateGroupRequest{
			Name:     "devs",
			LeaderID: validLeaderUUID1,
		})
		if !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("error = %v, want %v", err, access.ErrForbidden)
		}
	})

	t.Run("create group rejects malformed, empty, and nil leader UUIDs", func(t *testing.T) {
		svc := newService(&mockRepository{})
		invalidIDs := []string{
			"",
			"   ",
			"not-a-uuid",
			"11111111111111111111111111111111",         // missing hyphens
			"11111111-1111-1111-1111-11111111111g",     // non-hex char
			"1111111-11111-1111-1111-111111111111",     // wrong hyphen positions
			"11111111-1111-1111-1111-11111111111",      // too short
			"11111111-1111-1111-1111-1111111111111",    // too long
			"00000000-0000-0000-0000-000000000000",     // nil UUID
		}
		for _, id := range invalidIDs {
			_, err := svc.CreateGroup(ctx, adminPrincipal, api.CreateGroupRequest{Name: "devs", LeaderID: id})
			if !errors.Is(err, apperror.ErrInvalidInput) {
				t.Fatalf("leader_id %q: error = %v, want ErrInvalidInput", id, err)
			}
		}
	})

	t.Run("create group accepts uppercase UUID and allows one user to lead multiple groups", func(t *testing.T) {
		createdCount := 0
		repo := &mockRepository{
			createGroupFn: func(_ context.Context, name, leaderID string) (models.Group, error) {
				createdCount++
				return models.Group{ID: fmt.Sprintf("g-%d", createdCount), Name: name, LeaderID: &leaderID}, nil
			},
		}
		svc := newService(repo)

		g1, err := svc.CreateGroup(ctx, adminPrincipal, api.CreateGroupRequest{Name: "backend", LeaderID: upperLeaderUUID})
		if err != nil {
			t.Fatalf("create first group failed: %v", err)
		}
		g2, err := svc.CreateGroup(ctx, adminPrincipal, api.CreateGroupRequest{Name: "platform", LeaderID: upperLeaderUUID})
		if err != nil {
			t.Fatalf("create second group with same leader failed: %v", err)
		}
		if g1.LeaderID != upperLeaderUUID || g2.LeaderID != upperLeaderUUID {
			t.Fatalf("leaders = (%q, %q), want %q", g1.LeaderID, g2.LeaderID, upperLeaderUUID)
		}
	})

	t.Run("create group propagates inactive/missing leader and duplicate name errors", func(t *testing.T) {
		repo := &mockRepository{
			createGroupFn: func(_ context.Context, name, _ string) (models.Group, error) {
				if name == "dup" {
					return models.Group{}, fmt.Errorf("%w: already exists", apperror.ErrConflict)
				}
				return models.Group{}, fmt.Errorf("%w: leader must be an active user", apperror.ErrInvalidInput)
			},
		}
		svc := newService(repo)

		if _, err := svc.CreateGroup(ctx, adminPrincipal, api.CreateGroupRequest{Name: "inactive-test", LeaderID: validLeaderUUID1}); !errors.Is(err, apperror.ErrInvalidInput) {
			t.Fatalf("inactive leader error = %v, want ErrInvalidInput", err)
		}
		if _, err := svc.CreateGroup(ctx, adminPrincipal, api.CreateGroupRequest{Name: "dup", LeaderID: validLeaderUUID1}); !errors.Is(err, apperror.ErrConflict) {
			t.Fatalf("duplicate name error = %v, want ErrConflict", err)
		}
	})

	t.Run("list groups handles both assigned and legacy nil leader IDs", func(t *testing.T) {
		lid := validLeaderUUID1
		repo := &mockRepository{
			listGroupsFn: func(context.Context) ([]models.Group, error) {
				return []models.Group{
					{ID: "g1", Name: "with-leader", LeaderID: &lid},
					{ID: "g2", Name: "legacy-nil", LeaderID: nil},
				}, nil
			},
		}
		svc := newService(repo)
		groups, err := svc.ListGroups(ctx, adminPrincipal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(groups) != 2 || groups[0].LeaderID != validLeaderUUID1 || groups[1].LeaderID != "" {
			t.Fatalf("groups = %+v", groups)
		}
	})

	t.Run("update group rejects clearing leader, nil UUID, invalid name, and non-admin", func(t *testing.T) {
		svc := newService(&mockRepository{})

		empty := ""
		nilUUID := "00000000-0000-0000-0000-000000000000"
		tooLong := strings.Repeat("a", 101)

		for name, req := range map[string]api.UpdateGroupRequest{
			"clear leader to empty": {LeaderID: &empty},
			"nil leader UUID":       {LeaderID: &nilUUID},
			"empty name":            {Name: &empty},
			"name over 100 chars":   {Name: &tooLong},
		} {
			if _, err := svc.UpdateGroup(ctx, adminPrincipal, "g1", req); !errors.Is(err, apperror.ErrInvalidInput) {
				t.Fatalf("%s: error = %v, want ErrInvalidInput", name, err)
			}
		}

		svc.Access = &mockAccess{err: access.ErrForbidden}
		validLeader := validLeaderUUID2
		if _, err := svc.UpdateGroup(ctx, makePrincipal("user-2", false), "g1", api.UpdateGroupRequest{LeaderID: &validLeader}); !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("non-admin update error = %v, want ErrForbidden", err)
		}
	})

	t.Run("update group name-only preserves existing leader", func(t *testing.T) {
		existingLeader := validLeaderUUID1
		newName := "renamed-group"
		repo := &mockRepository{
			updateGroupFn: func(_ context.Context, groupID string, name, leaderID *string) (models.Group, error) {
				if leaderID != nil {
					t.Fatalf("expected nil leaderID for name-only update, got %v", *leaderID)
				}
				return models.Group{ID: groupID, Name: *name, LeaderID: &existingLeader}, nil
			},
		}
		svc := newService(repo)
		updated, err := svc.UpdateGroup(ctx, adminPrincipal, "g1", api.UpdateGroupRequest{Name: &newName})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updated.Name != newName || updated.LeaderID != existingLeader {
			t.Fatalf("updated = %+v", updated)
		}
	})

	t.Run("update group leader-only replaces leader", func(t *testing.T) {
		newLeader := validLeaderUUID2
		repo := &mockRepository{
			updateGroupFn: func(_ context.Context, groupID string, name, leaderID *string) (models.Group, error) {
				if name != nil {
					t.Fatalf("expected nil name for leader-only update, got %v", *name)
				}
				return models.Group{ID: groupID, Name: "devs", LeaderID: leaderID}, nil
			},
		}
		svc := newService(repo)
		updated, err := svc.UpdateGroup(ctx, adminPrincipal, "g1", api.UpdateGroupRequest{LeaderID: &newLeader})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updated.Name != "devs" || updated.LeaderID != newLeader {
			t.Fatalf("updated = %+v", updated)
		}
	})

	t.Run("remove group member propagates leader conflict", func(t *testing.T) {
		repo := &mockRepository{
			removeGroupMemberFn: func(context.Context, string, string, time.Time) error {
				return fmt.Errorf("%w: assign another leader before removing this member", apperror.ErrConflict)
			},
		}
		svc := newService(repo)
		err := svc.RemoveGroupMember(ctx, adminPrincipal, "g1", validLeaderUUID1, now)
		if !errors.Is(err, apperror.ErrConflict) {
			t.Fatalf("error = %v, want ErrConflict", err)
		}
	})

	t.Run("deactivate user who leads a group propagates leader conflict", func(t *testing.T) {
		repo := &mockRepository{
			getUserFn: func(context.Context, string) (models.User, error) {
				return models.User{ID: validLeaderUUID1, Username: "alice", IsActive: true}, nil
			},
			updateUserFn: func(context.Context, models.User, bool, time.Time) (models.User, error) {
				return models.User{}, fmt.Errorf("%w: assign another group leader before disabling this user", apperror.ErrConflict)
			},
		}
		svc := newService(repo)
		inactive := false
		_, err := svc.UpdateUser(ctx, adminPrincipal, validLeaderUUID1, api.UpdateUserRequest{IsActive: &inactive}, now)
		if !errors.Is(err, apperror.ErrConflict) {
			t.Fatalf("error = %v, want ErrConflict", err)
		}
	})
}

func TestGroupLeaderHandlerHTTPResponses(t *testing.T) {
	doRequest := func(h http.HandlerFunc, method, target, body string, pathVals map[string]string, principal *auth.Principal) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		for k, v := range pathVals {
			req.SetPathValue(k, v)
		}
		if principal != nil {
			req = req.WithContext(auth.SetValidatedPrincipalContext(req.Context(), *principal))
		}
		rr := httptest.NewRecorder()
		h(rr, req)
		return rr
	}

	t.Run("POST /groups maps validation, conflict, and success status codes", func(t *testing.T) {
		repo := &mockRepository{
			createGroupFn: func(_ context.Context, name, leaderID string) (models.Group, error) {
				if name == "dup" {
					return models.Group{}, fmt.Errorf("%w: duplicate group", apperror.ErrConflict)
				}
				return models.Group{ID: "g1", Name: name, LeaderID: &leaderID}, nil
			},
		}
		handler := &admin.Handler{Service: newService(repo)}

		// Missing leader_id -> 400
		rr := doRequest(handler.CreateGroup, http.MethodPost, "/api/v1/admin/groups", `{"name":"devs"}`, nil, &adminPrincipal)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("missing leader status = %d, want 400: %s", rr.Code, rr.Body.String())
		}

		// Duplicate name -> 409
		rr = doRequest(handler.CreateGroup, http.MethodPost, "/api/v1/admin/groups", `{"name":"dup","leader_id":"`+validLeaderUUID1+`"}`, nil, &adminPrincipal)
		if rr.Code != http.StatusConflict {
			t.Fatalf("duplicate status = %d, want 409: %s", rr.Code, rr.Body.String())
		}

		// Valid -> 201 with leader_id in response JSON
		rr = doRequest(handler.CreateGroup, http.MethodPost, "/api/v1/admin/groups", `{"name":"devs","leader_id":"`+validLeaderUUID1+`"}`, nil, &adminPrincipal)
		if rr.Code != http.StatusCreated {
			t.Fatalf("valid create status = %d, want 201: %s", rr.Code, rr.Body.String())
		}
		var resp struct {
			Data api.Group `json:"data"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Data.LeaderID != validLeaderUUID1 {
			t.Fatalf("response leader_id = %q, want %q", resp.Data.LeaderID, validLeaderUUID1)
		}
	})

	t.Run("PATCH /groups maps non-member leader (400), not found (404), duplicate name (409), and replacement (200)", func(t *testing.T) {
		repo := &mockRepository{
			updateGroupFn: func(_ context.Context, groupID string, name, leaderID *string) (models.Group, error) {
				if groupID == "missing" {
					return models.Group{}, apperror.ErrNotFound
				}
				if name != nil && *name == "dup" {
					return models.Group{}, fmt.Errorf("%w: duplicate group", apperror.ErrConflict)
				}
				if leaderID != nil && *leaderID == validLeaderUUID1 {
					return models.Group{}, fmt.Errorf("%w: leader must be a member of this group", apperror.ErrInvalidInput)
				}
				return models.Group{ID: groupID, Name: "devs", LeaderID: leaderID}, nil
			},
		}
		handler := &admin.Handler{Service: newService(repo)}

		// Clearing leader_id -> 400
		rr := doRequest(handler.UpdateGroup, http.MethodPatch, "/api/v1/admin/groups/g1", `{"leader_id":""}`, map[string]string{"groupId": "g1"}, &adminPrincipal)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("clear leader status = %d, want 400", rr.Code)
		}

		// Non-member leader -> 400
		rr = doRequest(handler.UpdateGroup, http.MethodPatch, "/api/v1/admin/groups/g1", `{"leader_id":"`+validLeaderUUID1+`"}`, map[string]string{"groupId": "g1"}, &adminPrincipal)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("non-member leader status = %d, want 400", rr.Code)
		}

		// Group not found -> 404
		rr = doRequest(handler.UpdateGroup, http.MethodPatch, "/api/v1/admin/groups/missing", `{"leader_id":"`+validLeaderUUID2+`"}`, map[string]string{"groupId": "missing"}, &adminPrincipal)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("not found status = %d, want 404", rr.Code)
		}

		// Duplicate name -> 409
		rr = doRequest(handler.UpdateGroup, http.MethodPatch, "/api/v1/admin/groups/g1", `{"name":"dup"}`, map[string]string{"groupId": "g1"}, &adminPrincipal)
		if rr.Code != http.StatusConflict {
			t.Fatalf("duplicate name status = %d, want 409", rr.Code)
		}

		// Valid leader replacement -> 200
		rr = doRequest(handler.UpdateGroup, http.MethodPatch, "/api/v1/admin/groups/g1", `{"leader_id":"`+validLeaderUUID2+`"}`, map[string]string{"groupId": "g1"}, &adminPrincipal)
		if rr.Code != http.StatusOK {
			t.Fatalf("valid replace status = %d, want 200: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("DELETE member and PATCH user return 409 when blocked by leader role", func(t *testing.T) {
		repo := &mockRepository{
			removeGroupMemberFn: func(context.Context, string, string, time.Time) error {
				return fmt.Errorf("%w: assign another leader before removing this member", apperror.ErrConflict)
			},
			getUserFn: func(context.Context, string) (models.User, error) {
				return models.User{ID: validLeaderUUID1, Username: "alice", IsActive: true}, nil
			},
			updateUserFn: func(context.Context, models.User, bool, time.Time) (models.User, error) {
				return models.User{}, fmt.Errorf("%w: assign another group leader before disabling this user", apperror.ErrConflict)
			},
		}
		handler := &admin.Handler{Service: newService(repo)}

		rr := doRequest(handler.RemoveGroupMember, http.MethodDelete, "/api/v1/admin/groups/g1/members/"+validLeaderUUID1, "", map[string]string{"groupId": "g1", "userId": validLeaderUUID1}, &adminPrincipal)
		if rr.Code != http.StatusConflict {
			t.Fatalf("remove leader status = %d, want 409: %s", rr.Code, rr.Body.String())
		}

		rr = doRequest(handler.UpdateUser, http.MethodPatch, "/api/v1/admin/users/"+validLeaderUUID1, `{"is_active":false}`, map[string]string{"userId": validLeaderUUID1}, &adminPrincipal)
		if rr.Code != http.StatusConflict {
			t.Fatalf("disable leader status = %d, want 409: %s", rr.Code, rr.Body.String())
		}
	})
}
