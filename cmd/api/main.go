package main

import (
	"log"
	"os"

	"github.com/asa/simple-disbursement-service-example/internal/config"
	"github.com/asa/simple-disbursement-service-example/internal/handler"
	"github.com/asa/simple-disbursement-service-example/internal/middleware"
	"github.com/asa/simple-disbursement-service-example/internal/model"
	"github.com/asa/simple-disbursement-service-example/internal/repository"
	"github.com/asa/simple-disbursement-service-example/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	// Load .env into the process env before reading config. Missing file is
	// fine (prod uses real env); godotenv never overwrites already-set vars.
	_ = godotenv.Load()

	cfg := config.Load()
	loggerZ := zerolog.New(os.Stdout).With().Timestamp().Logger()

	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}

	// Auto-migrate for local dev; production schema lives in migrations/.
	if err := db.AutoMigrate(
		&model.User{},
		&model.Disbursement{},
		&model.AuditLog{},
		&model.IdempotencyKey{},
		&repository.RefreshToken{},
	); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	if err := seedUsers(db); err != nil {
		log.Fatalf("seed users: %v", err)
	}

	userRepo := repository.NewUserRepo(db)
	disbRepo := repository.NewDisbursementRepo(db)
	auditRepo := repository.NewAuditRepo(db)
	refreshRepo := repository.NewRefreshTokenRepo(db)

	tokenManager := service.NewTokenManager(
		[]byte(cfg.JWTSecret), cfg.AccessTTL, cfg.RefreshTTL, refreshRepo, userRepo,
	)
	userSvc := service.NewUserService(userRepo, tokenManager)
	auditSvc := service.NewAuditService(auditRepo)
	disbSvc := service.NewDisbursementService(disbRepo, auditSvc)

	r := gin.New()
	r.Use(gin.Recovery(), middleware.RequestID(), middleware.StructuredLogger(&loggerZ))

	r.GET("/health", healthCheck(db))

	auth := handler.NewAuthHandler(userSvc)
	authGroup := r.Group("/auth")
	{
		authGroup.POST("/login", auth.Login)
		authGroup.POST("/refresh", auth.Refresh)
		authGroup.POST("/logout", auth.Logout)
	}

	disb := handler.NewDisbursementHandler(disbSvc)
	auditH := handler.NewAuditHandler(auditSvc)
	protected := r.Group("", middleware.Authenticate(tokenManager))
	{
		protected.GET("/disbursements", disb.List)
		protected.GET("/disbursements/:id", disb.Get)
		protected.POST("/disbursements", disb.Create)
		protected.PATCH("/disbursements/:id/status",
			middleware.RequireRoles(model.RoleAdmin, model.RoleSuperadmin), disb.UpdateStatus)
		protected.DELETE("/disbursements/:id",
			middleware.RequireRoles(model.RoleSuperadmin), disb.Delete)
		protected.GET("/audit-logs",
			middleware.RequireRoles(model.RoleSuperadmin), auditH.List)
	}

	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("run server: %v", err)
	}
}

func healthCheck(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.Ping() != nil {
			c.JSON(503, gin.H{"success": false, "error": "database unreachable"})
			return
		}
		c.JSON(200, gin.H{"success": true, "data": gin.H{"status": "ok"}})
	}
}

// seedUsers inserts the three staff accounts from the spec if absent.
func seedUsers(db *gorm.DB) error {
	seeds := []struct {
		username, password, role string
	}{
		{"superadmin", "superadmin123", string(model.RoleSuperadmin)},
		{"admin", "admin123", string(model.RoleAdmin)},
		{"operator", "operator123", string(model.RoleOperator)},
	}
	for _, s := range seeds {
		var count int64
		if err := db.Model(&model.User{}).Where("username = ?", s.username).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(s.password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if err := db.Create(&model.User{
			Username:     s.username,
			PasswordHash: string(hash),
			Role:         model.Role(s.role),
		}).Error; err != nil {
			return err
		}
		log.Printf("seeded user: %s", s.username)
	}
	return nil
}
