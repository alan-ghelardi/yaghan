package print

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	cpv1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
)

// NodesTable returns a Table to display nodes.
func NodesTable(getNodes GetItemsFunc[*cpv1alpha1.Node]) *Table[*cpv1alpha1.Node] {
	return &Table[*cpv1alpha1.Node]{
		GetItems: getNodes,
		Columns: []Column[*cpv1alpha1.Node]{
			NewColumn("Node ID", func(node *cpv1alpha1.Node) any {
				return node.Metadata.Id
			}),
			NewColumn("Created at", func(node *cpv1alpha1.Node) any {
				return node.Metadata.CreatedAt
			}, DurationFormatter),
			NewColumn("Last modified", func(node *cpv1alpha1.Node) any {
				return node.Metadata.LastModifiedAt
			}, DurationFormatter),
			NewColumn("Status", func(node *cpv1alpha1.Node) any {
				return humanFriendlyString(node.Status.Phase.String())
			}),
		},
	}
}

// SandboxesTable returns a Table to display sandboxes.
func SandboxesTable(getSandboxes GetItemsFunc[*cpv1alpha1.Sandbox]) *Table[*cpv1alpha1.Sandbox] {
	return &Table[*cpv1alpha1.Sandbox]{
		GetItems: getSandboxes,
		Columns: []Column[*cpv1alpha1.Sandbox]{
			NewColumn("Sandbox ID", func(s *cpv1alpha1.Sandbox) any {
				return s.GetMetadata().GetId()
			}),
			NewColumn("Namespace", func(s *cpv1alpha1.Sandbox) any {
				return s.GetMetadata().GetNamespace()
			}),
			NewColumn("Node ID", func(s *cpv1alpha1.Sandbox) any {
				return s.GetNode().GetId()
			}),
			NewColumn("Phase", func(s *cpv1alpha1.Sandbox) any {
				return humanFriendlyString(s.GetStatus().GetPhase().String())
			}),
			NewColumn("vCPU", func(s *cpv1alpha1.Sandbox) any {
				return s.GetResources().GetVcpuCount()
			}),
			NewColumn("Memory (MiB)", func(s *cpv1alpha1.Sandbox) any {
				return s.GetResources().GetMemoryMib()
			}),
			NewColumn("Created at", func(s *cpv1alpha1.Sandbox) any {
				return s.GetMetadata().GetCreatedAt()
			}, DurationFormatter),
		},
	}
}

// SnapshotsTable returns a Table to display snapshots.
func SnapshotsTable(getSnapshots GetItemsFunc[*cpv1alpha1.Snapshot]) *Table[*cpv1alpha1.Snapshot] {
	return &Table[*cpv1alpha1.Snapshot]{
		GetItems: getSnapshots,
		Columns: []Column[*cpv1alpha1.Snapshot]{
			NewColumn("Snapshot ID", func(s *cpv1alpha1.Snapshot) any {
				return s.GetMetadata().GetId()
			}),
			NewColumn("Namespace", func(s *cpv1alpha1.Snapshot) any {
				return s.GetMetadata().GetNamespace()
			}),
			NewColumn("Sandbox ID", func(s *cpv1alpha1.Snapshot) any {
				return s.GetSandbox().GetId()
			}),
			NewColumn("Created at", func(s *cpv1alpha1.Snapshot) any {
				return s.GetMetadata().GetCreatedAt()
			}, DurationFormatter),
			NewColumn("Description", func(s *cpv1alpha1.Snapshot) any {
				return s.GetMetadata().GetDescription()
			}),
		},
	}
}

func humanFriendlyString(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ToLower(s)
	return cases.Title(language.AmericanEnglish).String(s)
}
