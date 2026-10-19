package main

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestShowExactStoreRejectsIncompatibleModesBeforeReader(t *testing.T) {
	pinJSONOutput(t, true)
	for _, envelope := range []string{"0", "1"} {
		t.Run("envelope="+envelope, func(t *testing.T) {
			t.Setenv("BD_JSON_ENVELOPE", envelope)
			for _, flag := range []string{"current", "thread", "refs", "children", "watch", "as-of", "short", "long", "local-time", "comments-tail"} {
				t.Run(flag, func(t *testing.T) {
					cmd := &cobra.Command{}
					value := "true"
					switch flag {
					case "as-of":
						cmd.Flags().String(flag, "", "")
						value = "main"
					case "comments-tail":
						cmd.Flags().Int(flag, 0, "")
						value = "0"
					default:
						cmd.Flags().Bool(flag, false, "")
					}
					if err := cmd.Flags().Set(flag, value); err != nil {
						t.Fatal(err)
					}
					var err error
					stdout := captureStdout(t, func() error {
						err = runShowExactStore(cmd, context.Background(), []string{"bd-exact"})
						return nil
					})
					if code, reported := exitCodeFromError(err); !reported || code != 1 {
						t.Fatalf("incompatible mode did not return reported exit 1: %v", err)
					}
					if message := jsonErrorMessage(t, stdout); !strings.Contains(message, "--exact-store cannot be combined with --"+flag) {
						t.Fatalf("incompatible mode reached a reader: %v %s", err, stdout)
					}
				})
			}
		})
	}
}

func TestShowExactStoreRequiresJSONAndExplicitNonemptyIDs(t *testing.T) {
	for _, envelope := range []string{"0", "1"} {
		t.Run("envelope="+envelope, func(t *testing.T) {
			t.Setenv("BD_JSON_ENVELOPE", envelope)
			for _, tc := range []struct {
				name string
				json bool
				ids  []string
				want string
			}{
				{"text", false, []string{"bd-exact"}, "requires --json"},
				{"no-ids", true, nil, "at least one explicit issue ID"},
				{"empty", true, []string{""}, "nonempty exact issue IDs"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					pinJSONOutput(t, tc.json)
					var err error
					var message string
					if tc.json {
						stdout := captureStdout(t, func() error {
							err = runShowExactStore(&cobra.Command{}, context.Background(), tc.ids)
							return nil
						})
						message = jsonErrorMessage(t, stdout)
					} else {
						message = captureStderr(t, func() {
							err = runShowExactStore(&cobra.Command{}, context.Background(), tc.ids)
						})
					}
					if code, reported := exitCodeFromError(err); !reported || code != 1 {
						t.Fatalf("invalid exact read did not return reported exit 1: %v", err)
					}
					if !strings.Contains(message, tc.want) {
						t.Fatalf("invalid exact read reached a reader: %v %s", err, message)
					}
				})
			}
		})
	}
}
