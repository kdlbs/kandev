package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type recordedCommand struct {
	Args                  []string
	PID                   int
	StartedUTC, JoinedUTC string
	ExitCode              int
	Failure               string
	Stdout, Stderr        string
	process               *exec.Cmd
	files                 []*os.File
}

func newCommand(directory, name, selector string) *recordedCommand {
	return &recordedCommand{
		Args: commandArgs(selector), ExitCode: -1,
		Stdout: filepath.Join(directory, name+".jsonl"),
		Stderr: filepath.Join(directory, name+".stderr"),
	}
}

func (cmd *recordedCommand) Start() error {
	cmd.StartedUTC = time.Now().UTC().Format(time.RFC3339Nano)
	for _, path := range []string{cmd.Stdout, cmd.Stderr} {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			cmd.Failure = err.Error()
			return errors.Join(err, cmd.closeFiles())
		}
		cmd.files = append(cmd.files, file)
	}
	cmd.process = exec.Command("go", cmd.Args...)
	cmd.process.Stdout, cmd.process.Stderr = cmd.files[0], cmd.files[1]
	if err := cmd.process.Start(); err != nil {
		cmd.Failure = err.Error()
		return errors.Join(err, cmd.closeFiles())
	}
	cmd.PID = cmd.process.Process.Pid
	fmt.Fprintf(os.Stderr, "Started native Go PID %d; package timeout 25m; output %s\n", cmd.PID, cmd.Stdout)
	return nil
}

func (cmd *recordedCommand) Wait() error {
	err := cmd.process.Wait()
	cmd.JoinedUTC = time.Now().UTC().Format(time.RFC3339Nano)
	cmd.ExitCode = cmd.process.ProcessState.ExitCode()
	err = errors.Join(err, cmd.closeFiles())
	if err != nil {
		cmd.Failure = err.Error()
	}
	fmt.Fprintf(os.Stderr, "Joined native Go PID %d; actual exit %d\n", cmd.PID, cmd.ExitCode)
	return err
}

func (cmd *recordedCommand) closeFiles() error {
	var failures []error
	for _, file := range cmd.files {
		failures = append(failures, file.Close())
	}
	cmd.files = nil
	return errors.Join(failures...)
}

type runReport struct {
	Inventory inventory
	Cohorts   [2]cohort
	Commands  []*recordedCommand
	Complete  bool
	Failure   string
}

func runNative(directory string) (report runReport, result error) {
	listing := newCommand(directory, "inventory", "")
	report.Commands = append(report.Commands, listing)
	if err := joinCommands([]command{listing}); err != nil {
		return report, err
	}
	inv, err := inventoryFile(listing.Stdout)
	if err != nil {
		return report, err
	}
	report.Inventory = inv
	report.Cohorts, err = partition(inv)
	if err != nil {
		return report, err
	}
	var commands []command
	for index, group := range report.Cohorts {
		cmd := newCommand(directory, fmt.Sprintf("cohort-%d", index+1), group.Selector)
		report.Commands = append(report.Commands, cmd)
		commands = append(commands, cmd)
	}
	result = joinCommands(commands)
	for index, group := range report.Cohorts {
		err := coverageFile(report.Commands[index+1].Stdout, inv, group)
		result = errors.Join(result, err)
	}
	report.Complete = result == nil
	return report, result
}

func inventoryFile(path string) (inv inventory, result error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	return readInventory(file)
}

func coverageFile(path string, inv inventory, group cohort) (result error) {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	return validateCoverage(file, inv, group)
}

func publishReport(directory string, report runReport, result error) error {
	if result != nil {
		report.Failure = result.Error()
	}
	var failures []error
	for _, cmd := range report.Commands {
		for _, path := range []string{cmd.Stdout, cmd.Stderr} {
			fmt.Printf("Native diagnostics: %s\n", path)
			failures = append(failures, publishFile(path))
		}
	}
	if err := errors.Join(failures...); err != nil {
		report.Complete = false
		report.Failure = errors.Join(result, err).Error()
	}
	data, err := json.Marshal(report)
	if err != nil {
		return errors.Join(errors.Join(failures...), err)
	}
	err = os.WriteFile(filepath.Join(directory, "report.json"), data, 0o600)
	_, outputErr := fmt.Printf("Windows process cohort report: %s\n", data)
	return errors.Join(errors.Join(failures...), err, outputErr)
}

func publishFile(path string) (result error) {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	_, result = io.Copy(os.Stdout, file)
	return result
}

func main() {
	if runtime.GOOS != "windows" {
		fmt.Fprintln(os.Stderr, "Windows process cohorts require a native Windows runner")
		os.Exit(1)
	}
	directory, err := os.MkdirTemp(os.Getenv("RUNNER_TEMP"), "kandev-windows-process-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	report, err := runNative(directory)
	err = errors.Join(err, publishReport(directory, report, err))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
