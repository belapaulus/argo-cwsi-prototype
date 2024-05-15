package controller

import (
	"github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	//"github.com/argoproj/argo-workflows/v3/workflow/controller"
	"github.com/stretchr/testify/assert"
	"testing"
)

func checkGraph(t *testing.T, expected graph, actual graph) {
	for nodeName, node := range actual {
		assert.Equal(t, expected[nodeName].name, node.name)
		assert.Equal(t, expected[nodeName].pre, node.pre)
		assert.Equal(t, expected[nodeName].post, node.post)
	}
	// check that no expected nodes are missing
	for nodeName, node := range expected {
		assert.Equal(t, node.name, actual[nodeName].name)
		assert.Equal(t, node.pre, actual[nodeName].pre)
		assert.Equal(t, node.post, actual[nodeName].post)
	}
}

func TestParseDAGs(t *testing.T) {
	toTest := map[string]graph{
		"@testdata/simple-dag.yaml": {
			".A": {
				pre:  map[string]bool{},
				post: map[string]bool{".B": true, ".C": true},
				name: ".A",
			},
			".B": {
				pre:  map[string]bool{".A": true},
				post: map[string]bool{".D": true},
				name: ".B",
			},
			".C": {
				pre:  map[string]bool{".A": true},
				post: map[string]bool{".D": true},
				name: ".C",
			},
			".D": {
				pre:  map[string]bool{".B": true, ".C": true},
				post: map[string]bool{},
				name: ".D",
			},
		},
		"@testdata/nested-dag.yaml": {
			".A.A": {
				pre:  map[string]bool{},
				post: map[string]bool{".A.B": true, ".A.C": true},
				name: ".A.A",
			},
			".A.B": {
				pre:  map[string]bool{".A.A": true},
				post: map[string]bool{".B.D": true},
				name: ".A.B",
			},
			".A.C": {
				pre:  map[string]bool{".A.A": true},
				post: map[string]bool{".B.D": true},
				name: ".A.C",
			},
			".B.D": {
				pre:  map[string]bool{".A.B": true, ".A.C": true},
				post: map[string]bool{".B.E": true},
				name: ".B.D",
			},
			".B.E": {
				pre:  map[string]bool{".B.D": true},
				post: map[string]bool{},
				name: ".B.E",
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
