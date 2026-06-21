package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

// Prompter handles interactive terminal input during multi-step wizards.
type Prompter interface {
	Ask(prompts []platform.Prompt) (map[string]string, error)
	Confirm(prompt string) bool
}

// ConsolePrompter implements Prompter by reading from r and writing to w.
type ConsolePrompter struct {
	r io.Reader
	w io.Writer
}

func NewConsolePrompter(r io.Reader, w io.Writer) *ConsolePrompter {
	return &ConsolePrompter{r: r, w: w}
}

func (p *ConsolePrompter) Ask(prompts []platform.Prompt) (map[string]string, error) {
	reader := bufio.NewReader(p.r)
	answers := make(map[string]string, len(prompts))
	for _, pr := range prompts {
		label := pr.Label
		if pr.Default != "" {
			label = fmt.Sprintf("%s [%s]", label, pr.Default)
		}
		if _, err := fmt.Fprintf(p.w, "  %s: ", label); err != nil {
			return nil, fmt.Errorf("write prompt: %w", err)
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", pr.Key, err)
		}
		val := strings.TrimRight(line, "\r\n")
		if val == "" {
			val = pr.Default
		}
		if pr.Required && val == "" {
			return nil, fmt.Errorf("%q is required", pr.Label)
		}
		answers[pr.Key] = val
	}
	return answers, nil
}

func (p *ConsolePrompter) Confirm(prompt string) bool {
	_, _ = fmt.Fprint(p.w, "  "+prompt)
	reader := bufio.NewReader(p.r)
	line, _ := reader.ReadString('\n')
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
