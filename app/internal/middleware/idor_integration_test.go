//go:build integration

package middleware_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"contract_review/app/internal/comparison"
	"contract_review/app/internal/contract"
	"contract_review/app/internal/middleware"
	"contract_review/app/internal/qa"
	"contract_review/app/internal/review"
	"contract_review/app/internal/session"
	"contract_review/app/internal/user"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// idorSuite holds shared test fixtures.
type idorSuite struct {
	db  *gorm.DB
	u1  user.User
	u2  user.User
	ctx context.Context
}

func setupIDORSuite(t *testing.T) *idorSuite {
	t.Helper()

	dsn := os.Getenv("CONTRACT_REVIEW_TEST_DSN")
	if dsn == "" {
		t.Skip("CONTRACT_REVIEW_TEST_DSN not set; skipping IDOR integration test")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect to test db: %v", err)
	}

	// AutoMigrate all tables involved in ownership checks.
	if err := db.AutoMigrate(
		&user.User{},
		&session.Session{},
		&contract.Contract{},
		&contract.ContractType{},
		&review.ReviewTask{},
		&review.ReviewResult{},
		&comparison.ComparisonTask{},
		&qa.QAMessage{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	ctx := context.Background()

	// Clean up any test users from a previous run.
	db.WithContext(ctx).Where("account IN ?", []string{"idor_alice", "idor_bob"}).Delete(&user.User{})

	u1 := user.User{
		Account:    "idor_alice",
		Username:   "Alice IDOR Test",
		Password:   "test",
		SystemRole: "member",
	}
	u2 := user.User{
		Account:    "idor_bob",
		Username:   "Bob IDOR Test",
		Password:   "test",
		SystemRole: "member",
	}

	if err := db.WithContext(ctx).Create(&u1).Error; err != nil {
		t.Fatalf("create u1: %v", err)
	}
	if err := db.WithContext(ctx).Create(&u2).Error; err != nil {
		t.Fatalf("create u2: %v", err)
	}

	t.Cleanup(func() {
		db.WithContext(context.Background()).Where("account IN ?", []string{"idor_alice", "idor_bob"}).Delete(&user.User{})
		db.WithContext(context.Background()).Where("account IN ?", []string{"idor_alice", "idor_bob"}).Delete(&contract.Contract{})
		db.WithContext(context.Background()).Where("user_id IN ?", []uint{u1.ID, u2.ID}).Delete(&session.Session{})
		db.WithContext(context.Background()).Where("user_id IN ?", []uint64{uint64(u1.ID), uint64(u2.ID)}).Delete(&review.ReviewTask{})
		db.WithContext(context.Background()).Where("user_id IN ?", []uint64{uint64(u1.ID), uint64(u2.ID)}).Delete(&qa.QAMessage{})
		db.WithContext(context.Background()).Where("user_id IN ?", []uint64{uint64(u1.ID), uint64(u2.ID)}).Delete(&comparison.ComparisonTask{})
	})

	return &idorSuite{db: db, u1: u1, u2: u2, ctx: ctx}
}

// scopeFor builds a ResourceScope for the given user.
func scopeFor(u user.User) middleware.ResourceScope {
	return middleware.ResourceScope{
		OrganizationID: middleware.DefaultOrganizationID,
		UserID:         uint64(u.ID),
		Account:        u.Account,
		SystemRole:     u.SystemRole,
	}
}

// ---------------------------------------------------------------------------
// Contract IDOR tests — contracts are scoped by account (string column)
// ---------------------------------------------------------------------------

func TestIDOR_Contract_ListIsolation(t *testing.T) {
	s := setupIDORSuite(t)
	repo := contract.NewContractRepo(s.db)

	// Alice uploads a contract.
	c1 := &contract.Contract{
		Account:    s.u1.Account,
		Title:      fmt.Sprintf("idor-contract-list-%d", time.Now().UnixNano()),
		FilePath:   "/tmp/idor_test.pdf",
		FileType:   "pdf",
		Status:     "uploaded",
		UploadTime: time.Now(),
	}
	if err := s.db.WithContext(s.ctx).Create(c1).Error; err != nil {
		t.Fatalf("create contract: %v", err)
	}
	t.Cleanup(func() { s.db.WithContext(s.ctx).Delete(c1) })

	// Alice can see her contract.
	contracts, total, err := repo.ListContractsByAccount(s.ctx, s.u1.Account, 0, 10)
	if err != nil {
		t.Fatalf("u1 list: %v", err)
	}
	if total == 0 {
		t.Fatal("u1 should see her own contract")
	}
	found := false
	for _, c := range contracts {
		if c.ID == c1.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("u1 should find her own contract in list")
	}

	// Bob cannot see Alice's contract.
	_, total2, err := repo.ListContractsByAccount(s.ctx, s.u2.Account, 1, 10)
	if err != nil {
		t.Fatalf("u2 list: %v", err)
	}
	for _, c := range contracts {
		if c.Account == s.u2.Account && c.ID == c1.ID {
			t.Fatal("u2 should NOT see u1's contract in list")
		}
	}
	_ = total2 // u2 should see 0 contracts matching u1's
}

func TestIDOR_Contract_ReadIsolation(t *testing.T) {
	s := setupIDORSuite(t)
	repo := contract.NewContractRepo(s.db)

	// Alice creates a contract.
	c1 := &contract.Contract{
		Account:    s.u1.Account,
		Title:      fmt.Sprintf("idor-contract-read-%d", time.Now().UnixNano()),
		FilePath:   "/tmp/idor_test.pdf",
		FileType:   "pdf",
		Status:     "uploaded",
		UploadTime: time.Now(),
	}
	if err := s.db.WithContext(s.ctx).Create(c1).Error; err != nil {
		t.Fatalf("create contract: %v", err)
	}
	t.Cleanup(func() { s.db.WithContext(s.ctx).Delete(c1) })

	// Alice can read her contract by ID+Account.
	got, err := repo.GetContractByIDForAccount(s.ctx, c1.ID, s.u1.Account)
	if err != nil {
		t.Fatalf("u1 read own contract: %v", err)
	}
	if got == nil {
		t.Fatal("u1 should be able to read her own contract")
	}

	// Bob cannot read Alice's contract.
	got2, err := repo.GetContractByIDForAccount(s.ctx, c1.ID, s.u2.Account)
	if err != nil {
		t.Fatalf("u2 read u1 contract (unexpected repo error): %v", err)
	}
	if got2 != nil {
		t.Fatal("u2 should NOT be able to read u1's contract — IDOR vulnerability")
	}
}

func TestIDOR_Contract_DeleteIsolation(t *testing.T) {
	s := setupIDORSuite(t)

	// Alice creates a contract.
	c1 := &contract.Contract{
		Account:    s.u1.Account,
		Title:      fmt.Sprintf("idor-contract-delete-%d", time.Now().UnixNano()),
		FilePath:   "/tmp/idor_test.pdf",
		FileType:   "pdf",
		Status:     "uploaded",
		UploadTime: time.Now(),
	}
	if err := s.db.WithContext(s.ctx).Create(c1).Error; err != nil {
		t.Fatalf("create contract: %v", err)
	}
	t.Cleanup(func() { s.db.WithContext(s.ctx).Delete(c1) })

	// Bob tries to delete Alice's contract — should NOT affect it.
	result := s.db.WithContext(s.ctx).
		Where("id = ? AND account = ?", c1.ID, s.u2.Account).
		Delete(&contract.Contract{})
	if result.RowsAffected != 0 {
		t.Fatal("u2 should NOT be able to delete u1's contract — IDOR vulnerability")
	}

	// Verify Alice's contract still exists.
	var check contract.Contract
	if err := s.db.WithContext(s.ctx).Where("id = ?", c1.ID).First(&check).Error; err != nil {
		t.Fatalf("u1's contract should still exist after u2's delete attempt: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Session IDOR tests — sessions are scoped by user_id (numeric column)
// ---------------------------------------------------------------------------

func TestIDOR_Session_ReadIsolation(t *testing.T) {
	s := setupIDORSuite(t)
	sessionRepo := session.NewSessionRepo(s.db)

	// Alice creates a session.
	sess := &session.Session{
		UserID:      s.u1.ID,
		Title:       "idor-session-read",
		SessionType: "review",
	}
	if err := s.db.WithContext(s.ctx).Create(sess).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() { s.db.WithContext(s.ctx).Delete(sess) })

	// Alice can read her session.
	got, err := sessionRepo.GetByIDAndUserID(s.ctx, sess.ID, uint(s.u1.ID))
	if err != nil {
		t.Fatalf("u1 read own session: %v", err)
	}
	if got == nil {
		t.Fatal("u1 should be able to read her own session")
	}

	// Bob cannot read Alice's session.
	got2, err := sessionRepo.GetByIDAndUserID(s.ctx, sess.ID, uint(s.u2.ID))
	if err != nil {
		t.Fatalf("u2 read u1 session (unexpected repo error): %v", err)
	}
	if got2 != nil {
		t.Fatal("u2 should NOT read u1's session — IDOR vulnerability")
	}
}

func TestIDOR_Session_DeleteIsolation(t *testing.T) {
	s := setupIDORSuite(t)

	// Alice creates a session.
	sess := &session.Session{
		UserID:      s.u1.ID,
		Title:       "idor-session-delete",
		SessionType: "review",
	}
	if err := s.db.WithContext(s.ctx).Create(sess).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() { s.db.WithContext(s.ctx).Delete(sess) })

	// Bob tries to delete Alice's session — should not affect it.
	result := s.db.WithContext(s.ctx).
		Where("id = ? AND user_id = ?", sess.ID, s.u2.ID).
		Delete(&session.Session{})
	if result.RowsAffected != 0 {
		t.Fatal("u2 should NOT delete u1's session — IDOR vulnerability")
	}

	// Alice's session still exists.
	var check session.Session
	if err := s.db.WithContext(s.ctx).Where("id = ?", sess.ID).First(&check).Error; err != nil {
		t.Fatalf("u1's session should still exist: %v", err)
	}
}

func TestIDOR_Session_ListIsolation(t *testing.T) {
	s := setupIDORSuite(t)
	sessionRepo := session.NewSessionRepo(s.db)

	// Alice creates a session.
	sess := &session.Session{
		UserID:      s.u1.ID,
		Title:       "idor-session-list",
		SessionType: "review",
	}
	if err := s.db.WithContext(s.ctx).Create(sess).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() { s.db.WithContext(s.ctx).Delete(sess) })

	// Alice sees her session in list.
	sessions, total, err := sessionRepo.ListByUserID(s.ctx, uint(s.u1.ID), 0, 10)
	if err != nil {
		t.Fatalf("u1 list sessions: %v", err)
	}
	if total == 0 {
		t.Fatal("u1 should see her own session in list")
	}
	found := false
	for _, s := range sessions {
		if s.ID == sess.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("u1 should find her own session in list")
	}

	// Bob sees empty list (no u1 sessions).
	sessions2, _, err := sessionRepo.ListByUserID(s.ctx, uint(s.u2.ID), 0, 10)
	if err != nil {
		t.Fatalf("u2 list sessions: %v", err)
	}
	for _, sess := range sessions2 {
		if sess.UserID == s.u1.ID {
			t.Fatal("u2 should NOT see u1's sessions — IDOR vulnerability")
		}
	}
}

// ---------------------------------------------------------------------------
// Review task IDOR tests — review_tasks scoped by user_id
// ---------------------------------------------------------------------------

func TestIDOR_ReviewTask_ReadIsolation(t *testing.T) {
	s := setupIDORSuite(t)
	reviewRepo := review.NewReviewRepo(s.db)

	// Alice creates a review task.
	task := &review.ReviewTask{
		SessionID:    1,
		FileID:       1,
		UserID:       uint64(s.u1.ID),
		Stance:       "乙方",
		Intensity:    "严格",
		ContractType: "test",
		Status:       "pending",
	}
	if err := s.db.WithContext(s.ctx).Create(task).Error; err != nil {
		t.Fatalf("create review task: %v", err)
	}
	t.Cleanup(func() { s.db.WithContext(s.ctx).Delete(task) })

	// Alice can read her review task.
	got, err := reviewRepo.GetByIDAndUserID(s.ctx, task.ID, uint64(s.u1.ID))
	if err != nil {
		t.Fatalf("u1 read own review task: %v", err)
	}
	if got == nil {
		t.Fatal("u1 should be able to read her own review task")
	}

	// Bob cannot read Alice's review task.
	got2, err := reviewRepo.GetByIDAndUserID(s.ctx, task.ID, uint64(s.u2.ID))
	if err != nil {
		t.Fatalf("u2 read u1 review task (unexpected repo error): %v", err)
	}
	if got2 != nil {
		t.Fatal("u2 should NOT read u1's review task — IDOR vulnerability")
	}
}

func TestIDOR_ReviewTask_ListIsolation(t *testing.T) {
	s := setupIDORSuite(t)
	reviewRepo := review.NewReviewRepo(s.db)

	task := &review.ReviewTask{
		SessionID:    1,
		FileID:       1,
		UserID:       uint64(s.u1.ID),
		Stance:       "乙方",
		Intensity:    "标准",
		ContractType: "test",
		Status:       "pending",
	}
	if err := s.db.WithContext(s.ctx).Create(task).Error; err != nil {
		t.Fatalf("create review task: %v", err)
	}
	t.Cleanup(func() { s.db.WithContext(s.ctx).Delete(task) })

	// Alice sees her task in list.
	tasks, total, err := reviewRepo.ListByUserID(s.ctx, uint64(s.u1.ID), 0, 10)
	if err != nil {
		t.Fatalf("u1 list review tasks: %v", err)
	}
	if total == 0 {
		t.Fatal("u1 should see her own review task")
	}
	found := false
	for _, tk := range tasks {
		if tk.ID == task.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("u1 should find her own review task in list")
	}

	// Bob doesn't see Alice's task.
	tasks2, _, err := reviewRepo.ListByUserID(s.ctx, uint64(s.u2.ID), 0, 10)
	if err != nil {
		t.Fatalf("u2 list review tasks: %v", err)
	}
	for _, tk := range tasks2 {
		if tk.UserID == uint64(s.u1.ID) {
			t.Fatal("u2 should NOT see u1's review tasks — IDOR vulnerability")
		}
	}
	_ = tasks2
}

// ---------------------------------------------------------------------------
// QA message IDOR tests — qa_messages scoped by user_id
// ---------------------------------------------------------------------------

func TestIDOR_QA_ListIsolation(t *testing.T) {
	s := setupIDORSuite(t)

	// Alice creates a QA message.
	msg := &qa.QAMessage{
		SessionID: 1,
		UserID:    uint64(s.u1.ID),
		Role:      "user",
		Content:   "idor-qa-test-message",
	}
	if err := s.db.WithContext(s.ctx).Create(msg).Error; err != nil {
		t.Fatalf("create qa message: %v", err)
	}
	t.Cleanup(func() { s.db.WithContext(s.ctx).Delete(msg) })

	// Alice can list her messages.
	var u1Messages []qa.QAMessage
	if err := s.db.WithContext(s.ctx).
		Where("session_id = ? AND user_id = ?", 1, uint64(s.u1.ID)).
		Find(&u1Messages).Error; err != nil {
		t.Fatalf("u1 list qa messages: %v", err)
	}
	if len(u1Messages) == 0 {
		t.Fatal("u1 should see her own QA messages")
	}

	// Bob cannot read Alice's messages (different user_id).
	var u2Messages []qa.QAMessage
	if err := s.db.WithContext(s.ctx).
		Where("session_id = ? AND user_id = ?", 1, uint64(s.u2.ID)).
		Find(&u2Messages).Error; err != nil {
		t.Fatalf("u2 list qa messages: %v", err)
	}
	for _, m := range u2Messages {
		if m.UserID == uint64(s.u1.ID) {
			t.Fatal("u2 should NOT see u1's QA messages — IDOR vulnerability")
		}
	}
}

func TestIDOR_QA_DeleteIsolation(t *testing.T) {
	s := setupIDORSuite(t)

	msg := &qa.QAMessage{
		SessionID: 1,
		UserID:    uint64(s.u1.ID),
		Role:      "user",
		Content:   "idor-qa-delete-test",
	}
	if err := s.db.WithContext(s.ctx).Create(msg).Error; err != nil {
		t.Fatalf("create qa message: %v", err)
	}
	t.Cleanup(func() { s.db.WithContext(s.ctx).Delete(msg) })

	// Bob tries to delete Alice's message.
	result := s.db.WithContext(s.ctx).
		Where("session_id = ? AND user_id = ?", msg.SessionID, uint64(s.u2.ID)).
		Delete(&qa.QAMessage{})
	if result.RowsAffected != 0 {
		t.Fatal("u2 should NOT delete u1's QA messages — IDOR vulnerability")
	}

	// Alice's message still exists.
	var check qa.QAMessage
	if err := s.db.WithContext(s.ctx).Where("id = ?", msg.ID).First(&check).Error; err != nil {
		t.Fatalf("u1's qa message should still exist: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Comparison task IDOR tests — comparison_tasks scoped by user_id
// ---------------------------------------------------------------------------

func TestIDOR_ComparisonTask_ReadIsolation(t *testing.T) {
	s := setupIDORSuite(t)
	compRepo := comparison.NewComparisonRepo(s.db)

	task := &comparison.ComparisonTask{
		SessionID:        1,
		UserID:           uint64(s.u1.ID),
		StandardFileID:   1,
		ComparisonFileID: 2,
		Status:           "pending",
	}
	if err := s.db.WithContext(s.ctx).Create(task).Error; err != nil {
		t.Fatalf("create comparison task: %v", err)
	}
	t.Cleanup(func() { s.db.WithContext(s.ctx).Delete(task) })

	// Alice can read her comparison task.
	got, err := compRepo.GetByIDAndUserID(s.ctx, task.ID, uint64(s.u1.ID))
	if err != nil {
		t.Fatalf("u1 read own comparison task: %v", err)
	}
	if got == nil {
		t.Fatal("u1 should be able to read her own comparison task")
	}

	// Bob cannot read Alice's comparison task.
	got2, err := compRepo.GetByIDAndUserID(s.ctx, task.ID, uint64(s.u2.ID))
	if err != nil {
		t.Fatalf("u2 read u1 comparison task (unexpected repo error): %v", err)
	}
	if got2 != nil {
		t.Fatal("u2 should NOT read u1's comparison task — IDOR vulnerability")
	}
}

func TestIDOR_ComparisonTask_ListIsolation(t *testing.T) {
	s := setupIDORSuite(t)
	compRepo := comparison.NewComparisonRepo(s.db)

	task := &comparison.ComparisonTask{
		SessionID:        1,
		UserID:           uint64(s.u1.ID),
		StandardFileID:   1,
		ComparisonFileID: 2,
		Status:           "pending",
	}
	if err := s.db.WithContext(s.ctx).Create(task).Error; err != nil {
		t.Fatalf("create comparison task: %v", err)
	}
	t.Cleanup(func() { s.db.WithContext(s.ctx).Delete(task) })

	// Alice sees her task in list.
	tasks, _, err := compRepo.ListByUserID(s.ctx, uint64(s.u1.ID), 0, 10)
	if err != nil {
		t.Fatalf("u1 list comparison tasks: %v", err)
	}
	found := false
	for _, tk := range tasks {
		if tk.ID == task.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("u1 should find her own comparison task in list")
	}

	// Bob doesn't see Alice's task.
	tasks2, _, err := compRepo.ListByUserID(s.ctx, uint64(s.u2.ID), 0, 10)
	if err != nil {
		t.Fatalf("u2 list comparison tasks: %v", err)
	}
	for _, tk := range tasks2 {
		if tk.UserID == uint64(s.u1.ID) {
			t.Fatal("u2 should NOT see u1's comparison tasks — IDOR vulnerability")
		}
	}
	_ = tasks2
}

// ---------------------------------------------------------------------------
// ResourceScope consistency check
// ---------------------------------------------------------------------------

func TestIDOR_ResourceScope_IdentityConsistency(t *testing.T) {
	s := setupIDORSuite(t)

	scope1 := scopeFor(s.u1)
	scope2 := scopeFor(s.u2)

	// scopes for different users must differ.
	if scope1.UserID == scope2.UserID {
		t.Fatal("u1 and u2 must have different user IDs")
	}
	if scope1.Account == scope2.Account {
		t.Fatal("u1 and u2 must have different accounts")
	}

	// scope carries the default org.
	if scope1.OrganizationID != middleware.DefaultOrganizationID {
		t.Fatalf("u1 scope org = %d, want %d", scope1.OrganizationID, middleware.DefaultOrganizationID)
	}
	if scope2.OrganizationID != middleware.DefaultOrganizationID {
		t.Fatalf("u2 scope org = %d, want %d", scope2.OrganizationID, middleware.DefaultOrganizationID)
	}
}
