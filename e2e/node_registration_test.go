package e2e_test

import (
	"context"
	"time"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/e2e/internal/yag"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Node registration", func() {
	It("registers the daemon as a healthy node visible via `yag node list`",
		func(specCtx SpecContext) {
			ctx, cancel := context.WithTimeout(specCtx, 30*time.Second)
			defer cancel()

			var resp controlplanev1alpha1.ListNodesResponse
			Expect(yag.RunYAML(ctx, &resp, "node", "list")).To(Succeed())

			Expect(resp.GetNodes()).To(HaveLen(1),
				"expected exactly one registered node (BeforeSuite reset the table)")
			node := resp.GetNodes()[0]
			Expect(node.GetStatus().GetPhase()).
				To(Equal(controlplanev1alpha1.NodeStatus_PHASE_HEALTHY))
			Expect(node.GetMetadata().GetLastModifiedAt().AsTime()).
				To(BeTemporally(">=", suite.StartTime),
					"node's last-modified-at should be after the suite started")
		})
})
