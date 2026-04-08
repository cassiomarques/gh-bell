package main

import (
	"fmt"
	"os"

	"github.com/cassiomarques/gh-bell/internal/github"
)

func main() {
	client, err := github.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	notifications, err := client.ListNotifications(github.ListOptions{
		View:    github.ViewUnread,
		PerPage: 10,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(notifications) == 0 {
		fmt.Println("🔔 No unread notifications!")
		return
	}

	fmt.Printf("🔔 %d unread notification(s):\n\n", len(notifications))
	for _, n := range notifications {
		fmt.Printf("  %s  %-40s  [%s]  %s\n",
			n.Icon(),
			n.Subject.Title,
			n.ReasonLabel(),
			n.Repository.FullName,
		)
	}
}
