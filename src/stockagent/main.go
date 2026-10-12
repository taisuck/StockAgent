// Command stockagent는 국내 관심 종목 시세를 주기적으로 수집해
// VictoriaMetrics로 push한다.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/taisuck/StockAgent/internal/collector"
	"github.com/taisuck/StockAgent/internal/config"
	"github.com/taisuck/StockAgent/internal/kis"
	"github.com/taisuck/StockAgent/internal/logging"
	"github.com/taisuck/StockAgent/internal/market"
	"github.com/taisuck/StockAgent/internal/vmpush"
)

func main() {
	configPath := flag.String("config", "config.yml", "설정 파일 경로")
	logLevel := flag.String("log-level", "", "로그 레벨 (debug|info|warn|error). 비우면 설정 파일의 log.level")
	flag.Parse()

	if err := run(*configPath, *logLevel); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", err)
		os.Exit(1)
	}
}

func run(configPath, logLevel string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	if logLevel == "" {
		logLevel = cfg.Log.Level
	}
	level, err := logging.ParseLevel(logLevel)
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(market.TimeZone)
	if err != nil {
		return fmt.Errorf("타임존 %s 로드 실패: %w", market.TimeZone, err)
	}
	logFile, err := logging.NewDailyFile(cfg.Log.Dir, "stockagent", loc)
	if err != nil {
		return err
	}
	defer logFile.Close()
	// 표준 출력과 날짜별 파일에 같은 내용을 남긴다.
	// 파일 날짜와 어긋나지 않도록 레코드 시각도 KST로 찍는다.
	log := slog.New(slog.NewTextHandler(io.MultiWriter(os.Stdout, logFile), &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				a.Value = slog.TimeValue(a.Value.Time().In(loc))
			}
			return a
		},
	}))

	clock, err := market.NewClock(cfg.Market.Open, cfg.Market.Close)
	if err != nil {
		return err
	}

	kisClient := kis.New(cfg.KIS.BaseURL, cfg.KIS.AppKey, cfg.KIS.AppSecret, cfg.KIS.TokenCachePath, cfg.KIS.Timeout)
	gate := market.NewGate(clock, market.NewHolidayCache(kisClient, log))
	pusher := vmpush.New(cfg.Push.ImportURL, cfg.Push.Timeout)
	if cfg.Push.Username != "" {
		pusher.WithBasicAuth(cfg.Push.Username, cfg.Push.Password)
	}

	// SIGINT/SIGTERM을 받으면 진행 중인 주기를 끝내고 종료한다.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("StockAgent 시작",
		"symbols", len(cfg.Collect.Symbols),
		"interval", cfg.Collect.Interval,
		"session", cfg.Market.Open+"~"+cfg.Market.Close+" KST",
		"push", cfg.Push.ImportURL,
		"push_auth", cfg.Push.Username != "",
	)

	c := collector.New(cfg.Collect.Symbols, cfg.Collect.Interval, kisClient, pusher, gate, log)
	if err := c.Run(ctx); err != nil {
		return err
	}

	log.Info("StockAgent 종료")
	return nil
}
