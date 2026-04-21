package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.szostok.io/version/extension"

	"github.com/step-security/codeowners-validator/internal/check"
	"github.com/step-security/codeowners-validator/internal/envconfig"
	"github.com/step-security/codeowners-validator/internal/load"
	"github.com/step-security/codeowners-validator/internal/runner"
	"github.com/step-security/codeowners-validator/pkg/codeowners"
)

// Config holds the application configuration
type Config struct {
	RepositoryPath     string
	CheckFailureLevel  check.SeverityType `envconfig:"default=warning"`
	Checks             []string           `envconfig:"optional"`
	ExperimentalChecks []string           `envconfig:"optional"`
}

func main() {
	ctx, cancelFunc := WithStopContext(context.Background())
	defer cancelFunc()

	if err := NewRoot().ExecuteContext(ctx); err != nil {
		// error is already handled by `cobra`, we don't want to log it here as we will duplicate the message.
		// If needed, based on error type we can exit with different codes.
		//nolint:gocritic
		os.Exit(1)
	}
}

func exitOnError(err error) {
	if err != nil {
		logrus.Fatal(err)
	}
}

func validateSubscription() {
	isPublic := false

	if eventPath := os.Getenv("GITHUB_EVENT_PATH"); eventPath != "" {
		if eventData, err := os.ReadFile(eventPath); err == nil {
			var event struct {
				Repository struct {
					Private *bool `json:"private"`
				} `json:"repository"`
			}
			if err := json.Unmarshal(eventData, &event); err == nil {
				if event.Repository.Private != nil {
					isPublic = !*event.Repository.Private
				}
			}
		}
	}

	upstream := "mszostok/codeowners-validator"
	action := os.Getenv("GITHUB_ACTION_REPOSITORY")
	docsURL := "https://docs.stepsecurity.io/actions/stepsecurity-maintained-actions"

	fmt.Println()
	fmt.Println("\x1b[1;36mStepSecurity Maintained Action\x1b[0m")
	fmt.Printf("Secure drop-in replacement for %s\n", upstream)
	if isPublic {
		fmt.Println("\x1b[32m\u2713 Free for public repositories\x1b[0m")
	}
	fmt.Printf("\x1b[36mLearn more:\x1b[0m %s\n", docsURL)
	fmt.Println()

	if isPublic {
		return
	}

	serverURL := os.Getenv("GITHUB_SERVER_URL")
	if serverURL == "" {
		serverURL = "https://github.com"
	}

	body := map[string]string{"action": action}
	if serverURL != "https://github.com" {
		body["ghes_server"] = serverURL
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		fmt.Println("Timeout or API not reachable. Continuing to next step.")
		return
	}

	apiURL := fmt.Sprintf("https://agent.api.stepsecurity.io/v1/github/%s/actions/maintained-actions-subscription", os.Getenv("GITHUB_REPOSITORY"))

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(apiURL, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		fmt.Println("Timeout or API not reachable. Continuing to next step.")
		return
	}

	statusCode := resp.StatusCode
	resp.Body.Close()

	if statusCode == http.StatusForbidden {
		fmt.Printf("::error::\x1b[1;31mThis action requires a StepSecurity subscription for private repositories.\x1b[0m\n")
		fmt.Printf("::error::\x1b[31mLearn how to enable a subscription: %s\x1b[0m\n", docsURL)
		os.Exit(1)
	}
}

// WithStopContext returns a copy of parent with a new Done channel. The returned
// context's Done channel is closed on of SIGINT or SIGTERM signals.
func WithStopContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-ctx.Done():
		case <-sigCh:
			cancel()
		}
	}()

	return ctx, cancel
}

// NewRoot returns a root cobra.Command for the whole Agent utility.
func NewRoot() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "codeowners-validator",
		Short:        "Ensures the correctness of your CODEOWNERS file.",
		SilenceUsage: true,
		Run: func(cmd *cobra.Command, args []string) {
			validateSubscription()

			var cfg Config
			err := envconfig.Init(&cfg)
			exitOnError(err)

			log := logrus.New()

			// init checks
			checks, err := load.Checks(cmd.Context(), cfg.Checks, cfg.ExperimentalChecks)
			exitOnError(err)

			// init codeowners entries
			codeownersEntries, err := codeowners.NewFromPath(cfg.RepositoryPath)
			exitOnError(err)

			// run check runner
			absRepoPath, err := filepath.Abs(cfg.RepositoryPath)
			exitOnError(err)

			checkRunner := runner.NewCheckRunner(log, codeownersEntries, absRepoPath, cfg.CheckFailureLevel, checks...)
			checkRunner.Run(cmd.Context())

			if cmd.Context().Err() != nil {
				log.Error("Application was interrupted by operating system")
				os.Exit(2)
			}
			if checkRunner.ShouldExitWithCheckFailure() {
				os.Exit(3)
			}
		},
	}

	rootCmd.AddCommand(
		extension.NewVersionCobraCmd(),
	)

	return rootCmd
}
