package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/healthcheck"
	"github.com/go-logr/zapr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

func healthcheckCmd() *cobra.Command {
	hc := &cobra.Command{
		Short:        "Start health check probe",
		Use:          "healthcheck",
		RunE:         startHealthCheckServer,
		SilenceUsage: true,
	}

	hc.Flags().String("log-format", "console", "'console' or 'json'")
	hc.Flags().String("pvcs", "", "'pvc names delimited by comma'")
	hc.Flags().String("addr", fmt.Sprintf(":%d", healthcheck.Port), "listen address for server to bind")

	if err := viper.BindPFlags(hc.Flags()); err != nil {
		panic(err)
	}

	return hc
}

func startHealthCheckServer(cmd *cobra.Command, args []string) error {
	var (
		listenAddr = viper.GetString("addr")

		zlog   = zapLogger("info", viper.GetString("log-format"))
		logger = zapr.NewLogger(zlog)
	)
	defer func() { _ = zlog.Sync() }()

	var (
		pvcs = viper.GetString("pvcs")
		disk = healthcheck.DiskUsage(pvcs, healthcheck.Mount)
	)

	mux := http.NewServeMux()
	mux.Handle("/disk", disk)

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	var eg errgroup.Group
	eg.Go(func() error {
		logger.Info("Healthcheck server listening", "addr", listenAddr)
		return srv.ListenAndServe()
	})
	eg.Go(func() error {
		<-cmd.Context().Done()
		logger.Info("Healthcheck server shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	})

	return eg.Wait()
}
