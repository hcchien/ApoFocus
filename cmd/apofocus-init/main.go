package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hcchien/apofocus/internal/initjob"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type tagsFlag []string

func (f *tagsFlag) String() string     { return strings.Join(*f, ",") }
func (f *tagsFlag) Set(v string) error { *f = append(*f, v); return nil }
func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	db, err := sql.Open("pgx", required("DATABASE_URL"))
	fatal(err)
	defer db.Close()
	roots := splitPaths(required("APOFOCUS_IMPORT_ROOTS"))
	service := initjob.NewService(initjob.NewPostgresRepository(db), roots)
	ctx := context.Background()
	command := os.Args[1]
	args := os.Args[2:]
	switch command {
	case "create":
		create(ctx, service, args)
	case "status":
		need(args, 1)
		print(service.Get(ctx, args[0]))
	case "list":
		list(ctx, service, args)
	case "items":
		need(args, 1)
		print(service.Items(ctx, args[0], 500))
	case "pause":
		need(args, 1)
		fatal(service.Pause(ctx, args[0]))
		print(service.Get(ctx, args[0]))
	case "resume":
		need(args, 1)
		print(service.Resume(ctx, args[0]))
	case "cancel":
		need(args, 1)
		fatal(service.Cancel(ctx, args[0]))
		print(service.Get(ctx, args[0]))
	case "wait":
		need(args, 1)
		wait(ctx, service, args[0])
	default:
		usage()
		os.Exit(2)
	}
}
func create(ctx context.Context, s *initjob.Service, args []string) {
	flags := flag.NewFlagSet("create", flag.ExitOnError)
	source := flags.String("source", "", "folder or volume to catalog in place")
	project := flags.String("project", "", "shared project")
	recursive := flags.Bool("recursive", true, "scan subfolders")
	var tags tagsFlag
	flags.Var(&tags, "tag", "shared photographer tag; repeatable")
	_ = flags.Parse(args)
	print(s.Create(ctx, initjob.CreateInput{SourceRoot: *source, Project: *project, Tags: tags, Recursive: *recursive}))
}
func list(ctx context.Context, s *initjob.Service, args []string) {
	flags := flag.NewFlagSet("list", flag.ExitOnError)
	status := flags.String("status", "", "optional status")
	_ = flags.Parse(args)
	print(s.List(ctx, *status, 100))
}
func wait(ctx context.Context, s *initjob.Service, id string) {
	for {
		run, err := s.Get(ctx, id)
		fatal(err)
		printValue(run)
		if run.Terminal() || run.Status == "paused" {
			return
		}
		time.Sleep(5 * time.Second)
	}
}
func print(value any, err error) { fatal(err); printValue(value) }
func printValue(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	fatal(encoder.Encode(value))
}
func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "apofocus-init:", err)
		os.Exit(1)
	}
}
func required(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		fatal(errors.New(name + " is required"))
	}
	return value
}
func splitPaths(value string) []string {
	parts := strings.Split(value, string(filepath.ListSeparator))
	out := []string{}
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, part)
		}
	}
	return out
}
func need(args []string, count int) {
	if len(args) < count {
		usage()
		os.Exit(2)
	}
}
func usage() {
	fmt.Fprintln(os.Stderr, "Usage: apofocus-init {create --source PATH [--project NAME] [--tag TAG] | status ID | list | items ID | pause ID | resume ID | cancel ID | wait ID}")
}
