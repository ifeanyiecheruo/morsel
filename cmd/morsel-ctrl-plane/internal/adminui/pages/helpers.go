package pages

import (
	"fmt"
	"time"
)

// View model types used by templ components.

type AppRow struct {
	RepoSlug   string
	Name       string
	URL        string
	Status     string
	Hibernated bool
	Tier       string
	Cost       float64
	LastDeploy time.Time
}

type AppsPageData struct {
	Apps       []AppRow
	Total      int
	Repos      []string
	RepoFilter string
	Flash      string
	FlashError bool
}

type RepoRow struct {
	Slug       string
	AppCount   int64
	Cost       float64
	Tier       string
	LastDeploy time.Time
}

type ReposPageData struct {
	Repos      []RepoRow
	Flash      string
	FlashError bool
}

type ApprovalRow struct {
	ID             string
	Repo           string
	App            string
	Field          string
	CurrentValue   string
	RequestedValue string
	RequestedAt    time.Time
	ExpiresAt      time.Time
}

type ApprovalsPageData struct {
	Rows []ApprovalRow
}

type CostRepoRow struct {
	Slug        string
	Cost        float64
	PctOfBudget float64
	AppCount    int64
	Tier        string
}

type QuotaWarning struct {
	RepoSlug string
	AppName  string
	Cost     float64
	Limit    float64
}

type HibernateCandidate struct {
	RepoSlug  string
	AppName   string
	IdleSince time.Time
	Threshold string
	Cost      float64
}

type CostPageData struct {
	TotalCost  float64
	Budget     float64
	ByRepo     []CostRepoRow
	Warnings   []QuotaWarning
	Candidates []HibernateCandidate
}

type StatusPageData struct {
	ClusterHealthy      bool
	ControlPlaneHealthy bool
	FailedOps24h        int
	CertExpiringSoon    bool
	CertDomain          string
	PendingApprovals    int
	UpgradeAvailable    bool
	CurrentVersion      string
}

type StaleRow struct {
	ID         int64
	RepoSlug   string
	AppName    string
	LastDeploy time.Time
}

type StalePageData struct {
	Rows []StaleRow
}

type OperatorRow struct {
	GithubLogin string
	IsOperator  bool
	IsAdmin     bool
}

type OperatorsPageData struct {
	Rows          []OperatorRow
	Flash         string
	ViewerIsAdmin bool
}

// FormatCost formats a monthly cost as "$12.40/mo".
func FormatCost(v float64) string {
	return fmt.Sprintf("$%.2f/mo", v)
}

// FormatRelTime formats a time as a human-readable relative string.
func FormatRelTime(t time.Time) string {
	dur := time.Since(t)
	switch {
	case dur < time.Minute:
		return "just now"
	case dur < time.Hour:
		return fmt.Sprintf("%dm ago", int(dur.Minutes()))
	case dur < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(dur.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(dur.Hours()/24))
	}
}

// BarWidth returns the integer percentage (0–100) for a progress bar.
func BarWidth(value, total float64) int {
	if total <= 0 {
		return 0
	}
	pct := int(value / total * 100)
	if pct > 100 {
		return 100
	}
	return pct
}
