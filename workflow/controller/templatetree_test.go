package controller

import (
	"github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	//"github.com/argoproj/argo-workflows/v3/workflow/controller"
	"github.com/stretchr/testify/assert"
	"testing"
)

func checkGraph(t *testing.T, expected graph, actual graph) {
	for nodeName, node := range actual {
		assert.Equal(t, true, expected[nodeName] != nil)
		if expected[nodeName] != nil {
			assert.Equal(t, expected[nodeName].name, node.name)
			assert.Equal(t, expected[nodeName].pre, node.pre)
			assert.Equal(t, expected[nodeName].post, node.post)
			assert.Equal(t, expected[nodeName].nodeType, node.nodeType)
		}
	}
	// check that no expected nodes are missing
	for nodeName, node := range expected {
		assert.Equal(t, true, actual[nodeName] != nil)
		if actual[nodeName] != nil {
			assert.Equal(t, node.name, actual[nodeName].name)
			assert.Equal(t, node.pre, actual[nodeName].pre)
			assert.Equal(t, node.post, actual[nodeName].post)
			assert.Equal(t, node.nodeType, actual[nodeName].nodeType)
		}
	}
}

func TestParseDAGs(t *testing.T) {
	toTest := map[string]graph{
		"@testdata/simple-dag.yaml": {
			".A": {
				pre:      map[string]bool{},
				post:     map[string]bool{".B": true, ".C": true},
				name:     ".A",
				nodeType: "PROCESS",
			},
			".B": {
				pre:      map[string]bool{".A": true},
				post:     map[string]bool{".D": true},
				name:     ".B",
				nodeType: "PROCESS",
			},
			".C": {
				pre:      map[string]bool{".A": true},
				post:     map[string]bool{".D": true},
				name:     ".C",
				nodeType: "PROCESS",
			},
			".D": {
				pre:      map[string]bool{".B": true, ".C": true},
				post:     map[string]bool{},
				name:     ".D",
				nodeType: "PROCESS",
			},
		},
		"@testdata/nested-dag.yaml": {
			".A.A": {
				pre:      map[string]bool{},
				post:     map[string]bool{".A.B": true, ".A.C": true},
				name:     ".A.A",
				nodeType: "PROCESS",
			},
			".A.B": {
				pre:      map[string]bool{".A.A": true},
				post:     map[string]bool{".B.D": true},
				name:     ".A.B",
				nodeType: "PROCESS",
			},
			".A.C": {
				pre:      map[string]bool{".A.A": true},
				post:     map[string]bool{".B.D": true},
				name:     ".A.C",
				nodeType: "PROCESS",
			},
			".B.D": {
				pre:      map[string]bool{".A.B": true, ".A.C": true},
				post:     map[string]bool{".B.E": true},
				name:     ".B.D",
				nodeType: "PROCESS",
			},
			".B.E": {
				pre:      map[string]bool{".B.D": true},
				post:     map[string]bool{},
				name:     ".B.E",
				nodeType: "PROCESS",
			},
		},
		"@testdata/dag-with-suspend.yaml": {
			".A.A": {
				pre:      map[string]bool{},
				post:     map[string]bool{".A.B": true, ".B.D": true},
				name:     ".A.A",
				nodeType: "PROCESS",
			},
			".A.B": {
				pre:      map[string]bool{".A.A": true},
				post:     map[string]bool{".B.D": true},
				name:     ".A.B",
				nodeType: "PROCESS",
			},
			".B.D": {
				pre:      map[string]bool{".A.A": true, ".A.B": true},
				post:     map[string]bool{".B.E": true},
				name:     ".B.D",
				nodeType: "PROCESS",
			},
			".B.E": {
				pre:      map[string]bool{".B.D": true},
				post:     map[string]bool{},
				name:     ".B.E",
				nodeType: "PROCESS",
			},
		},
		"@testdata/dag-with-recursion.yaml": {
			".A": {
				pre:      map[string]bool{},
				post:     map[string]bool{".B": true, ".C.A": true},
				name:     ".A",
				nodeType: "PROCESS",
			},
			".B": {
				pre:      map[string]bool{".A": true},
				post:     map[string]bool{".D": true},
				name:     ".B",
				nodeType: "PROCESS",
			},
			".C.A": {
				pre:      map[string]bool{".A": true},
				post:     map[string]bool{".C.B": true, ".C.C": true},
				name:     ".C.A",
				nodeType: "PROCESS",
			},
			".C.B": {
				pre:      map[string]bool{".C.A": true},
				post:     map[string]bool{".C.D": true},
				name:     ".C.B",
				nodeType: "RECURSE",
			},
			".C.C": {
				pre:      map[string]bool{".C.A": true},
				post:     map[string]bool{".C.D": true},
				name:     ".C.C",
				nodeType: "RECURSE",
			},
			".C.D": {
				pre:      map[string]bool{".C.B": true, ".C.C": true},
				post:     map[string]bool{".D": true},
				name:     ".C.D",
				nodeType: "PROCESS",
			},
			".D": {
				pre:      map[string]bool{".B": true, ".C.D": true},
				post:     map[string]bool{".E": true},
				name:     ".D",
				nodeType: "PROCESS",
			},
			".E": {
				pre:      map[string]bool{".D": true},
				post:     map[string]bool{},
				name:     ".E",
				nodeType: "PROCESS",
			},
		},
		"@testdata/steps.yaml": {
			"[0].hello1": {
				pre:      map[string]bool{},
				post:     map[string]bool{"[1].hello2a": true, "[1].hello2b": true},
				name:     "[0].hello1",
				nodeType: "PROCESS",
			},
			"[1].hello2a": {
				pre:      map[string]bool{"[0].hello1": true},
				post:     map[string]bool{},
				name:     "[1].hello2a",
				nodeType: "PROCESS",
			},
			"[1].hello2b": {
				pre:      map[string]bool{"[0].hello1": true},
				post:     map[string]bool{},
				name:     "[1].hello2b",
				nodeType: "PROCESS",
			},
		},
	}
	for k, v := range toTest {
		wf := v1alpha1.MustUnmarshalWorkflow(k)
		woc := newWoc(*wf)
		graph := woc.parseTemplateTree()
		checkGraph(t, v, graph)
	}
}
