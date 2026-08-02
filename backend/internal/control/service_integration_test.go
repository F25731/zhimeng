package control

import (
	"os"
	"sync"
	"testing"

	"github.com/F25731/zhimeng/backend/internal/config"
	"github.com/F25731/zhimeng/backend/internal/security"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func integrationService(t *testing.T) (*Service, func()) {
	t.Helper()
	dsn := os.Getenv("CONTROL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CONTROL_TEST_DATABASE_URL is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(config.Config{
		RootDomain:          "test.invalid",
		MasterEncryptionKey: "0123456789abcdef0123456789abcdef",
		SessionSecret:       "0123456789abcdef0123456789abcdef",
		CardHashSecret:      "abcdef0123456789abcdef0123456789",
	}, db)
	return service, func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}
}

func insertProvisionSession(t *testing.T, service *Service, token string) (string, string) {
	t.Helper()
	codeID := uuid.NewString()
	sessionID := uuid.NewString()
	if err := service.db.Exec(`INSERT INTO provision_codes(id,code_prefix,code_hash,remark,status,max_sites,initial_version)
		VALUES (?, ?, ?, 'integration-test', 'reserved', 1, 'latest')`, codeID, "TEST-"+codeID[:4], uuid.NewString()).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.db.Exec(`INSERT INTO provision_sessions(id,provision_code_id,token_hash,status,expires_at)
		VALUES (?, ?, ?, 'active', now()+interval '10 minutes')`, sessionID, codeID, security.SHA256Hex(token)).Error; err != nil {
		t.Fatal(err)
	}
	return codeID, sessionID
}

func cleanupProvisionCode(t *testing.T, service *Service, codeID string) {
	t.Helper()
	_ = service.db.Exec(`DELETE FROM sites WHERE provision_code_id=?`, codeID).Error
	_ = service.db.Exec(`DELETE FROM provision_sessions WHERE provision_code_id=?`, codeID).Error
	_ = service.db.Exec(`DELETE FROM provision_codes WHERE id=?`, codeID).Error
}

func TestConcurrentDomainReservationAllowsOneSession(t *testing.T) {
	service, closeDB := integrationService(t)
	defer closeDB()
	tokenA, tokenB := uuid.NewString(), uuid.NewString()
	codeA, _ := insertProvisionSession(t, service, tokenA)
	codeB, _ := insertProvisionSession(t, service, tokenB)
	defer cleanupProvisionCode(t, service, codeA)
	defer cleanupProvisionCode(t, service, codeB)
	prefix := "t" + uuid.NewString()[:12]
	start := make(chan struct{})
	errorsByRequest := make(chan error, 2)
	var wg sync.WaitGroup
	for _, token := range []string{tokenA, tokenB} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			<-start
			_, err := service.ReserveDomain(value, prefix)
			errorsByRequest <- err
		}(token)
	}
	close(start)
	wg.Wait()
	close(errorsByRequest)
	successes := 0
	for err := range errorsByRequest {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected one successful reservation, got %d", successes)
	}
}

func TestVerifyReservedCodeResumesActiveSession(t *testing.T) {
	service, closeDB := integrationService(t)
	defer closeDB()
	plainCode := "SITE-TEST-RESU-ME01"
	codeID := uuid.NewString()
	oldToken := uuid.NewString()
	sessionID := uuid.NewString()
	if err := service.db.Exec(`INSERT INTO provision_codes(id,code_prefix,code_hash,remark,status,max_sites,initial_version,reserved_at)
		VALUES (?, 'SITE-TEST', ?, 'resume-integration-test', 'reserved', 1, 'latest', now())`, codeID, service.hashCode(plainCode)).Error; err != nil {
		t.Fatal(err)
	}
	defer cleanupProvisionCode(t, service, codeID)
	if err := service.db.Exec(`INSERT INTO provision_sessions(id,provision_code_id,token_hash,status,expires_at)
		VALUES (?, ?, ?, 'active', now()+interval '10 minutes')`, sessionID, codeID, security.SHA256Hex(oldToken)).Error; err != nil {
		t.Fatal(err)
	}
	prefix := "resume" + uuid.NewString()[:8]
	reservation, err := service.ReserveDomain(oldToken, prefix)
	if err != nil {
		t.Fatal(err)
	}
	newToken, resumed, job, err := service.VerifyCode(plainCode, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if newToken == "" || resumed.ID != sessionID || job != nil {
		t.Fatalf("unexpected resume result: token=%t session=%s job=%v", newToken != "", resumed.ID, job)
	}
	if resumed.Reservation == nil || resumed.Reservation.ID != reservation.ID || resumed.Reservation.Domain != reservation.Domain {
		t.Fatalf("reservation was not restored: %#v", resumed.Reservation)
	}
	if _, err := service.sessionID(oldToken); err == nil {
		t.Fatal("previous session token remained valid after resume")
	}
	if currentID, err := service.sessionID(newToken); err != nil || currentID != sessionID {
		t.Fatalf("new session token is invalid: id=%s err=%v", currentID, err)
	}
}

func TestConcurrentCreateProvisionReturnsSameJob(t *testing.T) {
	service, closeDB := integrationService(t)
	defer closeDB()
	token := uuid.NewString()
	codeID, _ := insertProvisionSession(t, service, token)
	defer cleanupProvisionCode(t, service, codeID)
	prefix := "t" + uuid.NewString()[:12]
	reservation, err := service.ReserveDomain(token, prefix)
	if err != nil {
		t.Fatal(err)
	}
	input := CreateSiteInput{Token: token, ReservationID: reservation.ID, Prefix: prefix, Name: "Integration Site", AdminUsername: "siteadmin", AdminPassword: "Password12345"}
	start := make(chan struct{})
	jobs := make(chan Job, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			job, createErr := service.CreateProvisionJob(input)
			jobs <- job
			errs <- createErr
		}()
	}
	close(start)
	wg.Wait()
	close(jobs)
	close(errs)
	for createErr := range errs {
		if createErr != nil {
			t.Fatal(createErr)
		}
	}
	var jobID string
	for job := range jobs {
		if jobID == "" {
			jobID = job.ID
		} else if job.ID != jobID {
			t.Fatalf("expected the same job, got %s and %s", jobID, job.ID)
		}
	}
}

func TestStaleLeaseCannotAdvanceJob(t *testing.T) {
	service, closeDB := integrationService(t)
	defer closeDB()
	jobID := uuid.NewString()
	if err := service.db.Exec(`INSERT INTO deployment_jobs(id,job_type,status,current_step,worker_id,lease_version,lease_until)
		VALUES (?, 'backup', 'running', 'pending', 'new-worker', 2, now()+interval '1 minute')`, jobID).Error; err != nil {
		t.Fatal(err)
	}
	defer service.db.Exec(`DELETE FROM deployment_jobs WHERE id=?`, jobID)
	stale := ClaimedJob{Job: Job{ID: jobID, WorkerID: "old-worker", LeaseVersion: 1}}
	if err := service.advance(stale, "processing", 20, "stale"); err == nil {
		t.Fatal("stale lease advanced the job")
	}
	var currentStep string
	if err := service.db.Raw(`SELECT current_step FROM deployment_jobs WHERE id=?`, jobID).Scan(&currentStep).Error; err != nil {
		t.Fatal(err)
	}
	if currentStep != "pending" {
		t.Fatalf("stale lease changed current step to %q", currentStep)
	}
}
