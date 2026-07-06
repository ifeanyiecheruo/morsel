package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	authnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/queue"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/health"
)

func newQueueCmd(ctx context.Context) *cobra.Command {
	var addr string
	var dataDir string
	var kubeconfigPath string

	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Run the queue service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dataDir == "" {
				return fmt.Errorf("--data-dir is required")
			}

			internalToken := os.Getenv("QUEUE_INTERNAL_TOKEN")
			if internalToken == "" {
				return fmt.Errorf("QUEUE_INTERNAL_TOKEN environment variable is required")
			}

			reporter, err := health.From(ctx)
			if err != nil {
				return fmt.Errorf("failed to get health reporter: %w", err)
			}

			queueHealth := reporter.NewComponent("queue", true)

			kubeClient := initializeKube(ctx, kubeconfigPath)

			reporter, receiver := health.NewReporter()
			ctx = health.With(ctx, reporter)
			go receiver.Run(ctx)

			h := &queueHandler{
				baseDir:       dataDir,
				kube:          kubeClient.Clientset(),
				internalToken: internalToken,
			}

			mux := http.NewServeMux()
			mux.HandleFunc("GET /livez", receiver.LivezHandler)
			mux.HandleFunc("GET /readyz", receiver.ReadyzHandler)
			mux.HandleFunc("GET /healthz", receiver.HealthzHandler)
			mux.HandleFunc("PUT /queues/{name}", h.createQueue)
			mux.HandleFunc("DELETE /queues/{name}", h.deleteQueue)
			mux.HandleFunc("GET /queues", h.listQueues)
			mux.HandleFunc("POST /queues/{name}/messages", h.enqueue)
			mux.HandleFunc("GET /queues/{name}/messages/next", h.dequeue)
			mux.HandleFunc("DELETE /queues/{name}/messages/{id}", h.ack)
			mux.HandleFunc("GET /queues/{name}/depth", h.depth)
			mux.HandleFunc("POST /internal/quota/{namespace}/{app}", h.setQuota)
			mux.HandleFunc("GET /internal/queues/{namespace}/{app}", h.idleStatus)

			queueHealth.Report(true, "ready")
			runServer(ctx, addr, 30*time.Second, func() *http.Server {
				return &http.Server{
					Handler:      mux,
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
				}
			})
			return nil
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8081", "HTTP listen address")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "root directory for queue SQLite files (required)")
	cmd.Flags().StringVar(&kubeconfigPath, "kubeconfig", "", "path to kubeconfig file")

	return cmd
}

// queueHandler holds shared dependencies for the queue service HTTP handlers.
type queueHandler struct {
	baseDir       string
	kube          kubernetes.Interface
	internalToken string
}

// queueFor creates a Queue scoped to ns. LocalQueue is cheap to construct —
// long-poll notifiers are package-level, so creating a new instance per
// request is safe and correct.
func (h *queueHandler) queueFor(ns string) queue.Queue {
	return queue.NewLocalQueue(h.baseDir, ns)
}

// — auth helpers —

// identify resolves the caller's Kubernetes namespace from the pod's projected
// service account token via TokenReview.
func (h *queueHandler) identify(r *http.Request) (string, error) {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" {
		return "", errors.New("missing bearer token")
	}
	tr, err := h.kube.AuthenticationV1().TokenReviews().Create(
		r.Context(),
		&authnv1.TokenReview{Spec: authnv1.TokenReviewSpec{Token: tok}},
		metav1.CreateOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("token review: %w", err)
	}
	if !tr.Status.Authenticated {
		return "", errors.New("not authenticated")
	}
	// Username format: system:serviceaccount:{namespace}:{sa-name}
	parts := strings.Split(tr.Status.User.Username, ":")
	if len(parts) != 4 || parts[0] != "system" || parts[1] != "serviceaccount" {
		return "", fmt.Errorf("unexpected username: %s", tr.Status.User.Username)
	}
	return parts[2], nil
}

func (h *queueHandler) checkInternalToken(r *http.Request) bool {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return tok != "" && tok == h.internalToken
}

// — response helpers —

func queueWriteJSON(ctx context.Context, w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		ctxlog.From(ctx).Error("write response", "err", err)
	}
}

func queueWriteError(w http.ResponseWriter, code int, errCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": errCode, "message": message})
}

// — handlers —

func (h *queueHandler) createQueue(w http.ResponseWriter, r *http.Request) {
	ns, err := h.identify(r)
	if err != nil {
		queueWriteError(w, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	if err := h.queueFor(ns).CreateQueue(r.Context(), r.PathValue("name")); err != nil {
		queueWriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *queueHandler) deleteQueue(w http.ResponseWriter, r *http.Request) {
	ns, err := h.identify(r)
	if err != nil {
		queueWriteError(w, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	if err := h.queueFor(ns).DeleteQueue(r.Context(), r.PathValue("name")); errors.Is(err, queue.ErrQueueNotFound) {
		queueWriteError(w, http.StatusNotFound, "queue_not_found", "queue does not exist")
		return
	} else if err != nil {
		queueWriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *queueHandler) listQueues(w http.ResponseWriter, r *http.Request) {
	ns, err := h.identify(r)
	if err != nil {
		queueWriteError(w, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	infos, err := h.queueFor(ns).ListQueues(r.Context(), parseIdleAfterDuration(r))
	if err != nil {
		queueWriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	type entry struct {
		Name  string `json:"name"`
		Depth int64  `json:"depth"`
		Idle  bool   `json:"idle"`
	}
	out := make([]entry, 0, len(infos))
	for _, info := range infos {
		out = append(out, entry{Name: info.Name, Depth: info.Depth, Idle: info.Idle})
	}
	queueWriteJSON(r.Context(), w, map[string]any{"queues": out})
}

func (h *queueHandler) enqueue(w http.ResponseWriter, r *http.Request) {
	senderNS, err := h.identify(r)
	if err != nil {
		queueWriteError(w, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	body, err := readRequestBody(r)
	if err != nil {
		queueWriteError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	q := h.queueFor(senderNS)
	if err := q.Enqueue(r.Context(), r.PathValue("name"), body, senderNS); errors.Is(err, queue.ErrQuotaExceeded) {
		used, _ := q.Usage(r.Context())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":      "queue_quota_exceeded",
			"used_bytes": used,
			"remedy":     "request a storage increase from your platform operator",
		})
		return
	} else if errors.Is(err, queue.ErrQueueNotFound) {
		queueWriteError(w, http.StatusNotFound, "queue_not_found", "queue does not exist")
		return
	} else if err != nil {
		queueWriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *queueHandler) dequeue(w http.ResponseWriter, r *http.Request) {
	ns, err := h.identify(r)
	if err != nil {
		queueWriteError(w, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	name := r.PathValue("name")
	q := h.queueFor(ns)

	msg, err := q.Dequeue(r.Context(), name, 0)
	if err != nil {
		queueWriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if msg == nil {
		waitCtx, cancel := context.WithTimeout(r.Context(), queue.LongPollTimeout)
		defer cancel()
		q.WaitForMessage(waitCtx, name)

		msg, err = q.Dequeue(r.Context(), name, 0)
		if err != nil {
			queueWriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	if msg == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	queueWriteJSON(r.Context(), w, map[string]any{
		"id":          msg.ID,
		"body":        base64.StdEncoding.EncodeToString(msg.Body),
		"enqueued_at": msg.EnqueuedAt.Format(time.RFC3339),
	})
}

func (h *queueHandler) ack(w http.ResponseWriter, r *http.Request) {
	ns, err := h.identify(r)
	if err != nil {
		queueWriteError(w, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	if err := h.queueFor(ns).Ack(r.Context(), r.PathValue("name"), r.PathValue("id")); errors.Is(err, queue.ErrQueueNotFound) {
		queueWriteError(w, http.StatusNotFound, "queue_not_found", "queue does not exist")
		return
	} else if err != nil {
		queueWriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *queueHandler) depth(w http.ResponseWriter, r *http.Request) {
	ns, err := h.identify(r)
	if err != nil {
		queueWriteError(w, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	d, err := h.queueFor(ns).Depth(r.Context(), r.PathValue("name"))
	if errors.Is(err, queue.ErrQueueNotFound) {
		queueWriteError(w, http.StatusNotFound, "queue_not_found", "queue does not exist")
		return
	} else if err != nil {
		queueWriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	queueWriteJSON(r.Context(), w, map[string]any{"depth": d})
}

func (h *queueHandler) setQuota(w http.ResponseWriter, r *http.Request) {
	if !h.checkInternalToken(r) {
		queueWriteError(w, http.StatusUnauthorized, "unauthorized", "invalid internal token")
		return
	}
	ns := r.PathValue("namespace") + "--" + r.PathValue("app")
	var body struct {
		QueueBytes int64 `json:"queue_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		queueWriteError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := h.queueFor(ns).SetQuota(r.Context(), body.QueueBytes); err != nil {
		queueWriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *queueHandler) idleStatus(w http.ResponseWriter, r *http.Request) {
	if !h.checkInternalToken(r) {
		queueWriteError(w, http.StatusUnauthorized, "unauthorized", "invalid internal token")
		return
	}
	ns := r.PathValue("namespace") + "--" + r.PathValue("app")
	infos, err := h.queueFor(ns).IdleStatus(r.Context(), parseIdleAfterDuration(r))
	if err != nil {
		queueWriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	type entry struct {
		Name  string `json:"name"`
		Depth int64  `json:"depth"`
		Idle  bool   `json:"idle"`
	}
	out := make([]entry, 0, len(infos))
	for _, info := range infos {
		out = append(out, entry{Name: info.Name, Depth: info.Depth, Idle: info.Idle})
	}
	queueWriteJSON(r.Context(), w, map[string]any{"queues": out})
}

// — utility —

func readRequestBody(r *http.Request) ([]byte, error) {
	const maxBody = 32 << 20 // 32 MB
	r.Body = http.MaxBytesReader(nil, r.Body, maxBody)
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}

func parseIdleAfterDuration(r *http.Request) time.Duration {
	s := r.URL.Query().Get("idle_after")
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}
