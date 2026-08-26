package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hcchien/apofocus/internal/batch"
)

type tagsFlag []string

func (f *tagsFlag) String() string { return strings.Join(*f, ",") }
func (f *tagsFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value != "" {
		*f = append(*f, value)
	}
	return nil
}

func main() {
	server := flag.String("server", "http://127.0.0.1:8080", "ApoFocus web API base URL")
	source := flag.String("source", "", "folder or mounted volume to scan")
	project := flag.String("project", "", "shared project; optional")
	recursive := flag.Bool("recursive", true, "scan nested folders")
	autoTags := flag.Bool("auto-tags", true, "run local OpenCLIP/CLAP automatic tags")
	wait := flag.Bool("wait", true, "poll and print progress until the job finishes")
	var tags tagsFlag
	var mediaTypes tagsFlag
	flag.Var(&tags, "tag", "shared tag; may be repeated")
	flag.Var(&mediaTypes, "media", "media type: photo, video, or audio; may be repeated (default: all)")
	flag.Parse()
	if strings.TrimSpace(*source) == "" {
		fmt.Fprintln(os.Stderr, "--source is required")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	client := &http.Client{Timeout: 15 * time.Second}
	job, err := submit(ctx, client, strings.TrimRight(*server, "/"), batch.CreateInput{SourceRoot: *source, Project: *project, Tags: tags, Recursive: *recursive, AutoTags: *autoTags, MediaTypes: mediaTypes})
	if err != nil {
		fmt.Fprintln(os.Stderr, "submit batch:", err)
		os.Exit(1)
	}
	fmt.Printf("batch job %s accepted\n", job.ID)
	if !*wait {
		return
	}
	if err := follow(ctx, client, strings.TrimRight(*server, "/"), job.ID); err != nil {
		fmt.Fprintln(os.Stderr, "follow batch:", err)
		os.Exit(1)
	}
}

func submit(ctx context.Context, client *http.Client, server string, input batch.CreateInput) (batch.Job, error) {
	body, _ := json.Marshal(input)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/api/v1/batch-jobs", bytes.NewReader(body))
	if err != nil {
		return batch.Job{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return batch.Job{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return batch.Job{}, fmt.Errorf("%s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	var job batch.Job
	err = json.NewDecoder(response.Body).Decode(&job)
	return job, err
}

func follow(ctx context.Context, client *http.Client, server, id string) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		job, err := getJob(ctx, client, server, id)
		if err != nil {
			return err
		}
		percent := 0
		if job.DiscoveredCount > 0 {
			percent = job.ProcessedCount * 100 / job.DiscoveredCount
		}
		fmt.Printf("\r%-22s %3d%%  %d/%d  success:%d  failed:%d", job.Status, percent, job.ProcessedCount, job.DiscoveredCount, job.SucceededCount, job.FailedCount)
		if job.Terminal() {
			fmt.Println()
			if job.Error != "" {
				return fmt.Errorf("job failed: %s", job.Error)
			}
			if job.FailedCount > 0 {
				fmt.Printf("inspect failures: %s/api/v1/batch-jobs/%s/items\n", server, id)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			fmt.Println()
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func getJob(ctx context.Context, client *http.Client, server, id string) (batch.Job, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server+"/api/v1/batch-jobs/"+id, nil)
	if err != nil {
		return batch.Job{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return batch.Job{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return batch.Job{}, fmt.Errorf("status request returned %s", response.Status)
	}
	var job batch.Job
	err = json.NewDecoder(response.Body).Decode(&job)
	return job, err
}
