package cli

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

// Prompter handles interactive terminal input during multi-step wizards.
type Prompter interface {
	Ask(prompts []platform.Prompt) (map[string]string, error)
	Confirm(prompt string) bool
}

// ConsolePrompter implements Prompter by reading from r and writing to w.
// A single bufio.Reader is shared across Ask and Confirm so that buffered-ahead
// bytes from one call are not lost before the next.
// When autoAcceptDefault is true, prompts with a default skip reading and
// Confirm returns true without input — equivalent to pressing Enter everywhere.
type ConsolePrompter struct {
	r                 *bufio.Reader
	w                 io.Writer
	autoAcceptDefault bool
}

func NewConsolePrompter(r io.Reader, w io.Writer) *ConsolePrompter {
	return &ConsolePrompter{r: bufio.NewReader(r), w: w}
}

func (p *ConsolePrompter) Ask(prompts []platform.Prompt) (map[string]string, error) {
	reader := p.r
	answers := make(map[string]string, len(prompts))
	for _, pr := range prompts {
		var val string
		var err error
		if len(pr.Choices) > 0 {
			val, err = p.askChoice(reader, pr)
		} else {
			val, err = p.askText(reader, pr)
		}
		if err != nil {
			return nil, err
		}
		if pr.Validate != nil {
			if err := pr.Validate(val); err != nil {
				return nil, fmt.Errorf("%s: %w", pr.Label, err)
			}
		}
		answers[pr.Key] = val
	}
	return answers, nil
}

// askText handles a free-form text prompt.
func (p *ConsolePrompter) askText(reader *bufio.Reader, pr platform.Prompt) (string, error) {
	if p.autoAcceptDefault && pr.Default != "" {
		return pr.Default, nil
	}
	label := pr.Label
	if pr.Default != "" {
		label = fmt.Sprintf("%s [%s]", label, pr.Default)
	}
	if _, err := fmt.Fprintf(p.w, "  %s: ", label); err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read %q: %w", pr.Key, err)
	}
	val := strings.TrimRight(line, "\r\n")
	if val == "" {
		val = pr.Default
	}
	if pr.Required && val == "" {
		return "", fmt.Errorf("%q is required", pr.Label)
	}
	return val, nil
}

// askChoice displays a numbered list and resolves the operator's digit to a value.
func (p *ConsolePrompter) askChoice(reader *bufio.Reader, pr platform.Prompt) (string, error) {
	if p.autoAcceptDefault && pr.Default != "" {
		return pr.Default, nil
	}
	if _, err := fmt.Fprintf(p.w, "  %s:\n", pr.Label); err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}
	defaultIdx := 1
	for i, c := range pr.Choices {
		if _, err := fmt.Fprintf(p.w, "    %d. %s\n", i+1, c); err != nil {
			return "", fmt.Errorf("write prompt: %w", err)
		}
		if c == pr.Default {
			defaultIdx = i + 1
		}
	}
	if _, err := fmt.Fprintf(p.w, "  Choice [%d]: ", defaultIdx); err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read %q: %w", pr.Key, err)
	}
	input := strings.TrimRight(line, "\r\n")
	if input == "" {
		return pr.Choices[defaultIdx-1], nil
	}
	n, err := strconv.Atoi(input)
	if err != nil || n < 1 || n > len(pr.Choices) {
		return "", fmt.Errorf("%s: enter a number between 1 and %d", pr.Label, len(pr.Choices))
	}
	return pr.Choices[n-1], nil
}

func (p *ConsolePrompter) Confirm(prompt string) bool {
	if p.autoAcceptDefault {
		return true
	}
	_, _ = fmt.Fprint(p.w, "  "+prompt)
	line, _ := p.r.ReadString('\n')
	ans := strings.TrimRight(line, "\r\n")
	return strings.EqualFold(ans, "y") || strings.EqualFold(ans, "yes")
}

func (p *ConsolePrompter) PrintPlan(plan platform.Plan) {
	_, _ = fmt.Fprintf(p.w, "\n  %s\n\n", plan.Summary)
	_, _ = fmt.Fprintln(p.w, "  Resources to be created:")
	for _, r := range plan.Resources {
		_, _ = fmt.Fprintf(p.w, "    • %s — %s\n", r.Name, r.Description)
	}
	_, _ = fmt.Fprintln(p.w)
}
