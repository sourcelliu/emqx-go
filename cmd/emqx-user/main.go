// Copyright 2023 The emqx-go Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main provides a CLI tool for managing EMQX-Go users and configuration
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/turtacn/emqx-go/pkg/config"
)

func main() {
	var configPath = flag.String("config", "config.yaml", "Path to configuration file")
	var command = flag.String("cmd", "", "Command: list, add, update, remove, enable, disable")
	var username = flag.String("user", "", "Username")
	var password = flag.String("pass", "", "Password")
	var algorithm = flag.String("algo", "bcrypt", "Password algorithm: plain, sha256, bcrypt")
	var enabled = flag.Bool("enabled", true, "User enabled status")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "EMQX-Go User Management Tool\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  list                  List all users\n")
		fmt.Fprintf(os.Stderr, "  add                   Add a new user\n")
		fmt.Fprintf(os.Stderr, "  update                Update an existing user\n")
		fmt.Fprintf(os.Stderr, "  remove                Remove a user\n")
		fmt.Fprintf(os.Stderr, "  enable                Enable a user\n")
		fmt.Fprintf(os.Stderr, "  disable               Disable a user\n")
		fmt.Fprintf(os.Stderr, "  generate              Generate sample config file\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -cmd=list\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -cmd=add -user=newuser -pass=newpass -algo=bcrypt\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -cmd=update -user=admin -pass=newpassword\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -cmd=remove -user=testuser\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -cmd=enable -user=admin\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -cmd=disable -user=guest\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -cmd=generate -config=new-config.yaml\n", os.Args[0])
	}

	flag.Parse()

	if *command == "" {
		flag.Usage()
		os.Exit(1)
	}

	switch *command {
	case "generate":
		generateConfig(*configPath)
	case "list":
		listUsers(*configPath)
	case "add":
		addUser(*configPath, *username, *password, *algorithm, *enabled)
	case "update":
		updateUser(*configPath, *username, *password, *algorithm, *enabled)
	case "remove":
		removeUser(*configPath, *username)
	case "enable":
		enableUser(*configPath, *username, true)
	case "disable":
		enableUser(*configPath, *username, false)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", *command)
		flag.Usage()
		os.Exit(1)
	}
}

func generateConfig(configPath string) {
	err := config.SaveConfig(config.DefaultConfig(), configPath)
	if err != nil {
		log.Fatalf("Failed to generate config file: %v", err)
	}
	fmt.Printf("✓ Sample configuration saved to %s\n", configPath)
}

func listUsers(configPath string) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	users := cfg.ListUsers()
	if len(users) == 0 {
		fmt.Println("No users configured")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "USERNAME\tALGORITHM\tENABLED\tPASSWORD")
	fmt.Fprintln(w, "--------\t---------\t-------\t--------")

	for _, user := range users {
		enabled := "✓"
		if !user.Enabled {
			enabled = "✗"
		}
		// Show password partially hidden for security
		password := user.Password
		if len(password) > 8 {
			password = password[:4] + "****" + password[len(password)-4:]
		} else if len(password) > 4 {
			password = password[:2] + "****" + password[len(password)-2:]
		} else {
			password = "****"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", user.Username, user.Algorithm, enabled, password)
	}
	w.Flush()
}

func addUser(configPath, username, password, algorithm string, enabled bool) {
	if username == "" {
		log.Fatal("Username is required")
	}
	if password == "" {
		log.Fatal("Password is required")
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	err = cfg.AddUser(username, password, algorithm, enabled)
	if err != nil {
		log.Fatalf("Failed to add user: %v", err)
	}

	err = config.SaveConfig(cfg, configPath)
	if err != nil {
		log.Fatalf("Failed to save config: %v", err)
	}

	enabledStr := "enabled"
	if !enabled {
		enabledStr = "disabled"
	}
	fmt.Printf("✓ User '%s' added successfully (algorithm: %s, status: %s)\n", username, algorithm, enabledStr)
}

func updateUser(configPath, username, password, algorithm string, enabled bool) {
	if username == "" {
		log.Fatal("Username is required")
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	err = cfg.UpdateUser(username, password, algorithm, enabled)
	if err != nil {
		log.Fatalf("Failed to update user: %v", err)
	}

	err = config.SaveConfig(cfg, configPath)
	if err != nil {
		log.Fatalf("Failed to save config: %v", err)
	}

	fmt.Printf("✓ User '%s' updated successfully\n", username)
}

func removeUser(configPath, username string) {
	if username == "" {
		log.Fatal("Username is required")
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	err = cfg.RemoveUser(username)
	if err != nil {
		log.Fatalf("Failed to remove user: %v", err)
	}

	err = config.SaveConfig(cfg, configPath)
	if err != nil {
		log.Fatalf("Failed to save config: %v", err)
	}

	fmt.Printf("✓ User '%s' removed successfully\n", username)
}

func enableUser(configPath, username string, enabled bool) {
	if username == "" {
		log.Fatal("Username is required")
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Find user and update only the enabled status
	found := false
	for _, user := range cfg.Broker.Auth.Users {
		if user.Username == username {
			err = cfg.UpdateUser(username, "", "", enabled)
			found = true
			break
		}
	}

	if !found {
		log.Fatalf("User '%s' not found", username)
	}

	if err != nil {
		log.Fatalf("Failed to update user: %v", err)
	}

	err = config.SaveConfig(cfg, configPath)
	if err != nil {
		log.Fatalf("Failed to save config: %v", err)
	}

	status := "enabled"
	if !enabled {
		status = "disabled"
	}
	fmt.Printf("✓ User '%s' %s successfully\n", username, status)
}
