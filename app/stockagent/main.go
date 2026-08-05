// Command stockagent는 국내 관심 종목 시세를 주기적으로 수집해
// VictoriaMetrics로 push한다.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/taisuck/StockAgent/internal/collector"
	"github.com/taisuck/StockAgent/internal/config"
	"github.com/taisuck/StockAgent/internal/kis"
	"github.com/taisuck/StockAgent/internal/market"
	"github.com/taisuck/StockAgent/internal/vmpush"
)

func main() {
	configPath := flag.String("config", "config.yml", "설정 파일 경로")
	logLevel := flag.String("log-level", "info", "로그 레벨 (debug|info|warn|error)")
	flag.Parse()

	if err := run(*configPath, *logLevel); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", err)
		os.Exit(1)
	}
}

func run(configPath, logLevel string) error {
	level, err := parseLevel(logLevel)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	clock, err := market.NewClock(cfg.Market.Open, cfg.Market.Close)
	if err != nil {
		return err
	}

	kisClient := kis.New(cfg.KIS.BaseURL, cfg.KIS.AppKey, cfg.KIS.AppSecret, cfg.KIS.TokenCachePath, cfg.KIS.Timeout)
	gate := market.NewGate(clock, market.NewHolidayCache(kisClient, log))
	pusher := vmpush.New(cfg.Push.ImportURL, cfg.Push.Timeout)

	// SIGINT/SIGTERM을 받으면 진행 중인 주기를 끝내고 종료한다.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("StockAgent 시작",
		"symbols", len(cfg.Collect.Symbols),
		"interval", cfg.Collect.Interval,
		"session", cfg.Market.Open+"~"+cfg.Market.Close+" KST",
		"push", cfg.Push.ImportURL,
	)

	c := collector.New(cfg.Collect.Symbols, cfg.Collect.Interval, kisClient, pusher, gate, log)
	if err := c.Run(ctx); err != nil {
		return err
	}

	log.Info("StockAgent 종료")
	return nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("알 수 없는 로그 레벨: %q", s)
	}
}
