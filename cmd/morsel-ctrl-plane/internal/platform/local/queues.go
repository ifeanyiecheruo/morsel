package local

import (
	"path/filepath"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/queue"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

func (lp *LocalPlatform) Queues(repoSlug, appName string) queue.Queue {
	return queue.NewLocalQueue(filepath.Join(localDataDir(), "queues"), kube.AppNamespace(repoSlug, appName))
}
