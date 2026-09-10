package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/paneacea/paneacea/internal/ipc"
	"github.com/paneacea/paneacea/internal/model"
	"github.com/paneacea/paneacea/internal/process"
	"io"
	"os"
	"strconv"
	"strings"
)

func Run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, err := fmt.Fprintln(out, `Paneacea CLI
  workspace list | create NAME ROOT | switch NAME_OR_ID
  tab create | rename TITLE | close
  pane split --right | --down
  agent list | resume | register JSON
  METHOD JSON       Send any runtime protocol request
Environment: PANEACEA_PANE_ID and PANEACEA_WORKSPACE_ID select context.`)
		return err
	}
	client, err := ipc.Ensure(context.Background())
	if err != nil {
		return err
	}
	defer client.Close()
	method := args[0]
	params := map[string]any{}
	if strings.Contains(method, ".") {
		if len(args) > 1 {
			if err = json.Unmarshal([]byte(args[1]), &params); err != nil {
				return err
			}
		}
	} else {
		if len(args) < 2 {
			return fmt.Errorf("expected resource and operation")
		}
		method = args[0] + "." + args[1]
		data, err := client.Call("state.get", nil)
		if err != nil {
			return err
		}
		var state model.State
		if err = json.Unmarshal(data, &state); err != nil {
			return err
		}
		workspaceID := os.Getenv("PANEACEA_WORKSPACE_ID")
		if workspaceID == "" {
			workspaceID = state.ActiveWorkspaceID
		}
		paneID := os.Getenv("PANEACEA_PANE_ID")
		tabID := ""
		if w := state.Workspace(workspaceID); w != nil {
			tabID = w.ActiveTabID
			_, t := state.Tab(tabID)
			if paneID == "" && t != nil {
				paneID = t.ActivePaneID
			}
		}
		params["workspaceId"] = workspaceID
		params["tabId"] = tabID
		params["paneId"] = paneID
		switch method {
		case "workspace.create":
			if len(args) != 4 {
				return fmt.Errorf("usage: workspace create NAME ROOT")
			}
			params["name"] = args[2]
			params["rootDirectory"] = args[3]
		case "workspace.switch":
			if len(args) != 3 {
				return fmt.Errorf("usage: workspace switch NAME_OR_ID")
			}
			found := ""
			for _, w := range state.Workspaces {
				if w.ID == args[2] || w.Name == args[2] {
					if found != "" {
						return fmt.Errorf("ambiguous workspace name; use its ID")
					}
					found = w.ID
				}
			}
			if found == "" {
				return fmt.Errorf("workspace not found")
			}
			params["workspaceId"] = found
		case "pane.split":
			if len(args) != 3 || (args[2] != "--right" && args[2] != "--down") {
				return fmt.Errorf("usage: pane split --right|--down")
			}
			params["orientation"] = "vertical"
			if args[2] == "--down" {
				params["orientation"] = "horizontal"
			}
		case "tab.rename":
			if len(args) != 3 {
				return fmt.Errorf("usage: tab rename TITLE")
			}
			params["title"] = args[2]
		case "agent.register":
			if len(args) != 3 {
				return fmt.Errorf("usage: agent register JSON")
			}
			var agent model.Agent
			if err = json.Unmarshal([]byte(args[2]), &agent); err != nil {
				return err
			}
			if agent.ProcessGeneration == "" {
				agent.ProcessGeneration = process.Generation(agent.RootPID)
			}
			params["agent"] = agent
		case "tab.move":
			if len(args) != 3 {
				return fmt.Errorf("usage: tab move INDEX")
			}
			index, err := strconv.Atoi(args[2])
			if err != nil {
				return err
			}
			params["index"] = index
		}
	}
	data, err := client.Call(method, params)
	if err != nil {
		return err
	}
	var formatted any
	if err = json.Unmarshal(data, &formatted); err != nil {
		return err
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(formatted)
}
