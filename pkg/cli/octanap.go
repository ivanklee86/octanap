package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/ivanklee86/octanap/pkg/client"
)

const TIMEOUT = 120

type Config struct {
	ServerAddr      string
	Insecure        bool
	AuthToken       string
	DryRun          bool
	LabelsAsStrings []string
	Labels          map[string]string
	SyncWindowsFile string
}

// Octanap is the logic/orchestrator.
type Octanap struct {
	*Config

	// Client
	ArgoCDClient          client.IArgoCDClient
	ArgoCDClientConnected bool

	// Allow swapping out stdout/stderr for testing.
	Out io.Writer
	Err io.Writer
}

func countCharacterOccurrences(s string, c rune) int {
	count := 0
	for _, char := range s {
		if char == c {
			count++
		}
	}
	return count
}

// New returns a new instance of octanap.
func New() *Octanap {
	config := Config{}

	for _, labelString := range config.LabelsAsStrings {
		if countCharacterOccurrences(labelString, '=') == 1 {
			kv := strings.Split(labelString, "=")

			if len(kv) == 2 {
				config.Labels[kv[0]] = kv[1]
			}
		}
	}

	return &Octanap{
		Config: &config,
		Out:    os.Stdout,
		Err:    os.Stdin,
	}
}

func NewWithConfig(config Config) *Octanap {
	return &Octanap{
		Config: &config,
		Out:    os.Stdout,
		Err:    os.Stdin,
	}
}

func (o *Octanap) Connect() {
	clientConfig := client.ArgoCDClientOptions{
		ServerAddr: o.Config.ServerAddr,
		Insecure:   o.Config.Insecure,
		AuthToken:  o.Config.AuthToken,
	}
	argocdClient, err := client.New(&clientConfig)
	if err != nil {
		o.Error(fmt.Sprintf("Creating ArgoCD client: %s", err.Error()))
	}

	o.ArgoCDClient = argocdClient
	o.ArgoCDClientConnected = true
}

func (o *Octanap) ClearSyncWindows() {
	ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*TIMEOUT)
	defer cancel()

	appProjects, err := o.ArgoCDClient.ListProjects(ctxTimeout)
	if err != nil {
		o.Error(fmt.Sprintf("Error fetching Projects: %s", err.Error()))
	}

	appProjectsToClear := filterProjects(appProjects, o.Config.Labels, true)

	for _, appProjectToClear := range appProjectsToClear {
		appProjectToClear.Spec.SyncWindows = nil
		_, err := o.ArgoCDClient.UpdateProject(ctxTimeout, appProjectToClear)
		if err != nil {
			o.Error(fmt.Sprintf("Error updating %s project: %s", appProjectToClear.ObjectMeta.Name, err))
		}
	}
}

func (o *Octanap) SetSyncWindows() {
	ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*TIMEOUT)
	defer cancel()

	syncWindowsToSet, err := readSyncWindowsFromFile(o.Config.SyncWindowsFile)
	if err != nil {
		o.Error(fmt.Sprintf("Unable to read SyncWindows file. %s", err.Error()))
	}

	appProjects, err := o.ArgoCDClient.ListProjects(ctxTimeout)
	if err != nil {
		o.Error(fmt.Sprintf("Error fetching Projects. %s", err.Error()))
	}

	appProjectsToUpdate := filterProjects(appProjects, o.Config.Labels, false)
	for _, appProjectToUpdate := range appProjectsToUpdate {
		var mergedSyncWindows v1alpha1.SyncWindows

		mergedSyncWindows = appProjectToUpdate.Spec.SyncWindows
		for _, syncWindow := range syncWindowsToSet {
			mergedSyncWindows = append(mergedSyncWindows, &syncWindow)
		}

		appProjectToUpdate.Spec.SyncWindows = mergedSyncWindows

		_, err := o.ArgoCDClient.UpdateProject(ctxTimeout, appProjectToUpdate)
		if err != nil {
			o.Error(fmt.Sprintf("Error updating %s project: %s", appProjectToUpdate.ObjectMeta.Name, err))
		}
	}
}
