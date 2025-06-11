package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// LogsCommand returns the logs command
func LogsCommand() *cli.Command {
	return &cli.Command{
		Name:      "logs",
		Usage:     "Stream logs from ManagedRoost resources",
		ArgsUsage: "NAME",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "follow",
				Aliases: []string{"f"},
				Usage:   "Follow log output",
			},
			&cli.Int64Flag{
				Name:  "tail",
				Usage: "Number of lines to show from the end",
				Value: 100,
			},
			&cli.StringFlag{
				Name:  "container",
				Usage: "Container name (for multi-container pods)",
			},
			&cli.BoolFlag{
				Name:  "previous",
				Usage: "Show logs from previous container instance",
			},
			&cli.DurationFlag{
				Name:  "since",
				Usage: "Show logs newer than this duration",
			},
		},
		Action: func(c *cli.Context) error {
			return runLogs(c)
		},
	}
}

// runLogs executes the logs command
func runLogs(c *cli.Context) error {
	config := GetGlobalConfig()
	ctx := context.Background()

	if c.NArg() == 0 {
		return fmt.Errorf("resource name is required")
	}

	name := c.Args().Get(0)
	namespace := config.GetCurrentNamespace()

	// Get the ManagedRoost resource
	var roost roostv1alpha1.ManagedRoost
	if err := config.K8sClient.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, &roost); err != nil {
		return fmt.Errorf("failed to get ManagedRoost: %w", err)
	}

	// Find pods associated with this roost
	pods, err := getAssociatedPods(ctx, &roost)
	if err != nil {
		return fmt.Errorf("failed to get associated pods: %w", err)
	}

	if len(pods) == 0 {
		fmt.Println("No pods found for this ManagedRoost")
		return nil
	}

	// If multiple pods, show logs from all unless container is specified
	if len(pods) > 1 && c.String("container") == "" {
		fmt.Printf("Found %d pods. Showing logs from all pods:\n", len(pods))
		for i, pod := range pods {
			if i > 0 {
				fmt.Println("\n" + strings.Repeat("=", 50))
			}
			fmt.Printf("Pod: %s\n", pod.Name)
			fmt.Println(strings.Repeat("=", 50))
			if err := streamPodLogs(ctx, &pod, c); err != nil {
				fmt.Printf("Error streaming logs from pod %s: %v\n", pod.Name, err)
			}
		}
		return nil
	}

	// Stream logs from first pod or specified container
	return streamPodLogs(ctx, &pods[0], c)
}

// getAssociatedPods finds pods associated with a ManagedRoost
func getAssociatedPods(ctx context.Context, roost *roostv1alpha1.ManagedRoost) ([]corev1.Pod, error) {
	config := GetGlobalConfig()

	// Create label selector for Helm release
	labelSelector := labels.Set{
		"app.kubernetes.io/managed-by": "Helm",
		"app.kubernetes.io/instance":   roost.Name,
	}.AsSelector()

	var podList corev1.PodList
	listOptions := &client.ListOptions{
		Namespace:     roost.Namespace,
		LabelSelector: labelSelector,
	}

	if err := config.K8sClient.List(ctx, &podList, listOptions); err != nil {
		return nil, err
	}

	return podList.Items, nil
}

// streamPodLogs streams logs from a specific pod
func streamPodLogs(ctx context.Context, pod *corev1.Pod, c *cli.Context) error {
	config := GetGlobalConfig()

	// Build log options
	logOptions := &corev1.PodLogOptions{
		Follow:    c.Bool("follow"),
		TailLines: func() *int64 { v := c.Int64("tail"); return &v }(),
		Previous:  c.Bool("previous"),
	}

	if container := c.String("container"); container != "" {
		logOptions.Container = container
	}

	if since := c.Duration("since"); since > 0 {
		sinceTime := metav1.NewTime(time.Now().Add(-since))
		logOptions.SinceTime = &sinceTime
	}

	// Get log stream
	req := config.RESTClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOptions)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("failed to get log stream: %w", err)
	}
	defer podLogs.Close()

	// Stream logs to stdout
	_, err = io.Copy(os.Stdout, podLogs)
	if err != nil {
		return fmt.Errorf("failed to stream logs: %w", err)
	}

	return nil
}
