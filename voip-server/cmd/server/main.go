package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"voip-server/internal/api"
	"voip-server/internal/audio"
	"voip-server/internal/config"
	"voip-server/internal/db"
	"voip-server/internal/llm"
	"voip-server/internal/sip"
	"voip-server/internal/state"
	"voip-server/internal/stt"
	"voip-server/internal/tts"
	"voip-server/internal/transfer"
	wshandler "voip-server/internal/websocket"
)

func main() {
	// Load configuration
	cfg, err := config.Load(config.GetConfigPath())
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize logger
	logger, err := initLogger(&cfg.Logging, cfg.Server.Environment)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	logger.Info("Starting VoIP Server",
		zap.String("version", cfg.Server.Version),
		zap.String("environment", cfg.Server.Environment))

	// Initialize CockroachDB (primary durable datastore)
	crdb, err := db.New(&cfg.CockroachDB, logger)
	if err != nil {
		logger.Fatal("Failed to initialize CockroachDB", zap.Error(err))
	}
	if crdb != nil {
		defer crdb.Close()
	}

	// Initialize Redis client (caching layer + pub-sub)
	redisClient := initRedis(&cfg.Redis, logger)

	// Initialize call manager (CockroachDB primary, Redis cache)
	callManager := state.NewCallManager(crdb, redisClient)

	// Initialize SIP server
	var sipServer *sip.Server
	if cfg.Server.SIP.Enabled {
		sipServer, err = sip.NewServer(&cfg.Server.SIP, callManager, logger)
		if err != nil {
			logger.Fatal("Failed to create SIP server", zap.Error(err))
		}
	}

	// Initialize STT manager
	sttManager, err := stt.NewManager(&cfg.STT, logger)
	if err != nil {
		logger.Fatal("Failed to create STT manager", zap.Error(err))
	}

	// Initialize TTS manager
	ttsManager, err := tts.NewManager(&cfg.TTS, logger)
	if err != nil {
		logger.Fatal("Failed to create TTS manager", zap.Error(err))
	}

	// Initialize LLM manager
	llmManager, err := llm.NewManager(&cfg.LLM, logger)
	if err != nil {
		logger.Fatal("Failed to create LLM manager", zap.Error(err))
	}

	// Initialize transfer engine
	transferEngine := transfer.NewEngine(&cfg.Transfer, callManager, logger)
	transferEngine.Start(context.Background())

	// Initialize audio pipeline
	audioPipeline := audio.NewPipeline(
		&cfg.Audio,
		sttManager,
		ttsManager,
		llmManager,
		transferEngine,
		callManager,
		logger,
	)

	// Initialize WebSocket server
	wsServer := wshandler.NewServer(audioPipeline, callManager, logger)

	// Initialize API handler
	apiHandler := api.NewHandler(
		&cfg.Server,
		callManager,
		sipServer,
		audioPipeline,
		sttManager,
		ttsManager,
		llmManager,
		transferEngine,
		logger,
	)

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      cfg.Server.Name,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	})

	// Enable CORS
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Setup routes
	apiHandler.SetupRoutes(app)
	
	// WebSocket endpoint
	wsServer.SetupRoutes(app)

	// Webhook handlers
	webhookHandlers := api.NewWebhookHandlers(callManager, logger)
	app.Post("/webhooks/twilio", webhookHandlers.HandleTwilio)
	app.Post("/webhooks/signalwire", webhookHandlers.HandleSignalWire)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start services
	if cfg.Server.SIP.Enabled && sipServer != nil {
		if err := sipServer.Start(ctx); err != nil {
			logger.Fatal("Failed to start SIP server", zap.Error(err))
		}
		logger.Info("SIP server started",
			zap.String("address", fmt.Sprintf("%s:%d", cfg.Server.SIP.Host, cfg.Server.SIP.Port)))
	}

	// Start REST API
	if cfg.Server.REST.Enabled {
		go func() {
			addr := fmt.Sprintf("%s:%d", cfg.Server.REST.Host, cfg.Server.REST.Port)
			logger.Info("REST API starting", zap.String("address", addr))
			if err := app.Listen(addr); err != nil {
				logger.Fatal("Failed to start REST API", zap.Error(err))
			}
		}()
	}

	// Start cleanup goroutine
	go cleanupRoutine(callManager, audioPipeline, logger)

	logger.Info("VoIP Server started successfully")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	logger.Info("Received shutdown signal", zap.String("signal", sig.String()))

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown REST API
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logger.Error("Failed to shutdown REST API", zap.Error(err))
	}

	// Stop transfer engine
	transferEngine.Stop()

	// Stop SIP server
	if sipServer != nil {
		if err := sipServer.Stop(); err != nil {
			logger.Error("Failed to stop SIP server", zap.Error(err))
		}
	}

	// Cleanup remaining sessions
	callManager.Cleanup()
	audioPipeline.Cleanup()

	logger.Info("VoIP Server stopped")
}

// initLogger initializes the logger
func initLogger(cfg *config.LoggingConfig, environment string) (*zap.Logger, error) {
	level := zap.InfoLevel
	switch cfg.Level {
	case "debug":
		level = zap.DebugLevel
	case "info":
		level = zap.InfoLevel
	case "warn":
		level = zap.WarnLevel
	case "error":
		level = zap.ErrorLevel
	}

	// Validate output path - default to stdout if empty or invalid
	outputPath := cfg.Output
	if outputPath == "" || outputPath == "stdout" {
		outputPath = "stdout"
	}

	config := zap.Config{
		Level:       zap.NewAtomicLevelAt(level),
		Development: environment == "development",
		Encoding:    cfg.Format,
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "timestamp",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{outputPath},
		ErrorOutputPaths: []string{"stderr"},
	}

	return config.Build()
}

// initRedis initializes the Redis client
func initRedis(cfg *config.RedisConfig, logger *zap.Logger) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.Database,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	})

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		logger.Warn("Redis connection failed, continuing without session persistence",
			zap.Error(err))
		return nil
	}

	logger.Info("Redis connected successfully")
	return client
}

// cleanupRoutine periodically cleans up resources
func cleanupRoutine(callManager *state.CallManager, audioPipeline *audio.Pipeline, logger *zap.Logger) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		callManager.Cleanup()
		audioPipeline.Cleanup()
		logger.Debug("Cleanup completed",
			zap.Int("active_calls", len(callManager.GetActiveSessions())),
			zap.Int("audio_sessions", audioPipeline.GetActiveSessions()))
	}
}
