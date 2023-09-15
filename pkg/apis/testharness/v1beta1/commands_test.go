package v1beta1

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateCommandOutput(t *testing.T) {
	tests := []struct {
		name         string
		cmdOutput    CommandOutput
		stdoutOutput strings.Builder
		stderrOutput strings.Builder
		wantErr      bool
	}{
		{
			name: "stdout matches exactly",
			cmdOutput: CommandOutput{
				Stdout: &ExpectedOutput{
					MatchType:     MatchEquals,
					ExpectedValue: "Hello, World!",
				},
			},
			stdoutOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Hello, World!")
				return b
			}(),
			wantErr: false,
		},
		{
			name: "stdout does not match exactly",
			cmdOutput: CommandOutput{
				Stdout: &ExpectedOutput{
					MatchType:     MatchEquals,
					ExpectedValue: "Hello",
				},
			},
			stdoutOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Hello, World!")
				return b
			}(),
			wantErr: true,
		},
		{
			name: "stdout and stderr with contains match",
			cmdOutput: CommandOutput{
				Stdout: &ExpectedOutput{
					MatchType:     MatchContains,
					ExpectedValue: "World",
				},
				Stderr: &ExpectedOutput{
					MatchType:     MatchContains,
					ExpectedValue: "Err",
				},
			},
			stdoutOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Hello, World!")
				return b
			}(),
			stderrOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("An Error Occurred")
				return b
			}(),
			wantErr: false,
		},
		{
			name: "stdout wildcard match",
			cmdOutput: CommandOutput{
				Stdout: &ExpectedOutput{
					MatchType:     MatchWildcard,
					ExpectedValue: "Hello, *!",
				},
			},
			stdoutOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Hello, World!")
				return b
			}(),
			wantErr: false,
		},
		{
			name: "stdout matches but stderr fails",
			cmdOutput: CommandOutput{
				Stdout: &ExpectedOutput{
					MatchType:     MatchEquals,
					ExpectedValue: "Hello, World!",
				},
				Stderr: &ExpectedOutput{
					MatchType:     MatchEquals,
					ExpectedValue: "Error",
				},
			},
			stdoutOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Hello, World!")
				return b
			}(),
			stderrOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Different Error")
				return b
			}(),
			wantErr: true,
		},
		{
			name: "stderr contains but stdout fails",
			cmdOutput: CommandOutput{
				Stderr: &ExpectedOutput{
					MatchType:     MatchContains,
					ExpectedValue: "Error",
				},
				Stdout: &ExpectedOutput{
					MatchType:     MatchEquals,
					ExpectedValue: "Hello",
				},
			},
			stdoutOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Hello, World!")
				return b
			}(),
			stderrOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Different Error")
				return b
			}(),
			wantErr: true,
		},
		{
			name: "stdout wildcard match with missing pattern",
			cmdOutput: CommandOutput{
				Stdout: &ExpectedOutput{
					MatchType:     MatchWildcard,
					ExpectedValue: "Hi, *!",
				},
			},
			stdoutOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Hello, World!")
				return b
			}(),
			wantErr: true,
		},
		{
			name: "stderr wildcard match",
			cmdOutput: CommandOutput{
				Stderr: &ExpectedOutput{
					MatchType:     MatchWildcard,
					ExpectedValue: "*Error*",
				},
			},
			stderrOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("This is an Error message!")
				return b
			}(),
			wantErr: false,
		},
		{
			name: "stdout contains and stderr wildcard match",
			cmdOutput: CommandOutput{
				Stdout: &ExpectedOutput{
					MatchType:     MatchContains,
					ExpectedValue: "universe",
				},
				Stderr: &ExpectedOutput{
					MatchType:     MatchWildcard,
					ExpectedValue: "*Error*",
				},
			},
			stdoutOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Hello, universe!")
				return b
			}(),
			stderrOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Some Error here!")
				return b
			}(),
			wantErr: false,
		},
		{
			name: "stderr equals but stdout wildcard fails",
			cmdOutput: CommandOutput{
				Stderr: &ExpectedOutput{
					MatchType:     MatchEquals,
					ExpectedValue: "Some Error here!",
				},
				Stdout: &ExpectedOutput{
					MatchType:     MatchWildcard,
					ExpectedValue: "Greetings, *",
				},
			},
			stdoutOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Hello, World!")
				return b
			}(),
			stderrOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Some Error here!")
				return b
			}(),
			wantErr: true,
		},
		{
			name: "stdout contains with no stderr",
			cmdOutput: CommandOutput{
				Stdout: &ExpectedOutput{
					MatchType:     MatchContains,
					ExpectedValue: "planet",
				},
			},
			stdoutOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Hello, planet Earth!")
				return b
			}(),
			wantErr: false,
		},
		{
			name: "stderr equals with no stdout",
			cmdOutput: CommandOutput{
				Stderr: &ExpectedOutput{
					MatchType:     MatchEquals,
					ExpectedValue: "Error: File not found.",
				},
			},
			stderrOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Error: File not found.")
				return b
			}(),
			wantErr: false,
		},
		{
			name: "stdout contains but missing in actual output",
			cmdOutput: CommandOutput{
				Stdout: &ExpectedOutput{
					MatchType:     MatchContains,
					ExpectedValue: "Mars",
				},
			},
			stdoutOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Hello, World!")
				return b
			}(),
			wantErr: true,
		},
		{
			name: "stderr wildcard does not match",
			cmdOutput: CommandOutput{
				Stderr: &ExpectedOutput{
					MatchType:     MatchWildcard,
					ExpectedValue: "Critical*Error*",
				},
			},
			stderrOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("This is a simple Error message.")
				return b
			}(),
			wantErr: true,
		},
		{
			name: "Empty Expected Output",
			cmdOutput: CommandOutput{
				Stderr: &ExpectedOutput{
					MatchType:     MatchContains,
					ExpectedValue: "",
				},
			},
			stderrOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Empty Expected Output")
				return b
			}(),
			wantErr: false,
		},
		{
			name: "defualt match type is not provided",
			cmdOutput: CommandOutput{
				Stdout: &ExpectedOutput{
					MatchType:     "",
					ExpectedValue: "Hello, World!",
				},
			},
			stdoutOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Hello, World!")
				return b
			}(),
			wantErr: false,
		},
		{
			name: "random match type is not provided",
			cmdOutput: CommandOutput{
				Stdout: &ExpectedOutput{
					MatchType:     "abc",
					ExpectedValue: "Hello, World!",
				},
			},
			stdoutOutput: func() strings.Builder {
				b := strings.Builder{}
				b.WriteString("Hello, World!")
				return b
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmdOutput.ValidateCommandOutput(tt.stdoutOutput, tt.stderrOutput)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
