package graph

import "github.com/zyvorai/scout/internal/model"

func AssignWaves(inv model.Inventory, assessments []model.Assessment) []model.Assessment {
	idx := make(map[string]int, len(assessments))
	status := make(map[string]string, len(assessments))
	for i := range assessments {
		idx[assessments[i].VMID] = i
		status[assessments[i].VMID] = assessments[i].Status
	}

	adj := make(map[string][]string)
	for _, vm := range inv.VMs {
		adj[vm.ID] = nil
	}
	for _, e := range inv.Connections {
		adj[e.From] = append(adj[e.From], e.To)
		adj[e.To] = append(adj[e.To], e.From)
	}
	visited := map[string]bool{}
	wave := 0
	for _, vm := range inv.VMs {
		if visited[vm.ID] || status[vm.ID] == "blocked" {
			continue
		}
		wave++
		queue := []string{vm.ID}
		visited[vm.ID] = true
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			if i, ok := idx[id]; ok {
				assessments[i].Wave = wave
			}
			for _, n := range adj[id] {
				if visited[n] || status[n] == "blocked" {
					continue
				}
				visited[n] = true
				queue = append(queue, n)
			}
		}
	}
	return assessments
}

func Build(inv model.Inventory, assessments []model.Assessment) model.Graph {
	byID := make(map[string]model.Assessment, len(assessments))
	for _, a := range assessments {
		byID[a.VMID] = a
	}
	g := model.Graph{}
	for _, vm := range inv.VMs {
		a := byID[vm.ID]
		g.Nodes = append(g.Nodes, model.GraphNode{ID: vm.ID, Label: vm.Name, Status: a.Status, Wave: a.Wave})
	}
	for _, c := range inv.Connections {
		g.Edges = append(g.Edges, model.GraphEdge{From: c.From, To: c.To, Protocol: c.Protocol, Port: c.Port})
	}
	return g
}
