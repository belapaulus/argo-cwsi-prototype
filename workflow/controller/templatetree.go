package controller

import (
	"fmt"
	"hash/fnv"

	"github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
)

type parseNode struct {
	// set of nodes that this node depends on
	pre map[string]bool
	// set of nodes that depend on this node
	post         map[string]bool
	name         string
	templateName string
	// unique identifier for the scheduler
	// TODO derive id from name
	id uint32
}

type graph map[string]*parseNode

type parseCtx struct {
	// map to look up templates by name
	templates map[string]*v1alpha1.Template
	// map that contains all nodes by name
	nodes graph
	// node counter currently used to create id
	numNodes int
	woc      *wfOperationCtx
}

func getNodeIDFromName(name string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return h.Sum32()
}

func (woc *wfOperationCtx) parseTemplateTree() graph {
	// create parse context
	pc := &parseCtx{
		templates: make(map[string]*v1alpha1.Template),
		nodes:     make(graph),
		numNodes:  0,
		woc:       woc,
	}
	// store all workflow templates in map
	for i, template := range woc.execWf.Spec.Templates {
		pc.templates[template.Name] = &woc.execWf.Spec.Templates[i]
	}
	// add root node
	rootNode := pc.newNode(woc.execWf.ObjectMeta.Name, woc.execWf.Spec.Entrypoint)
	pc.nodes[rootNode.name] = rootNode
	// the template tree is parsed by repeatedly replacing the inner nodes
	// of the template tree with their children with the function expandNode()
	pc.expandNode(rootNode.name)
	return pc.nodes
}

func (pc *parseCtx) newNode(nodeName string, templateName string) *parseNode {
	return &parseNode{
		pre:          make(map[string]bool),
		post:         make(map[string]bool),
		name:         nodeName,
		templateName: templateName,
		id:           getNodeIDFromName(nodeName),
	}
}

func (pc *parseCtx) expandNode(nodeName string) {
	// TODO im Moment probiere ich kanten zu knoten hinzuzufügen, die es noch gar nicht gibt.
	// ich glaube ich muss immer erst die knoten erstellen und dann auf den knoten parse template rufen
	// TODO document set usage (no set[key] = false)
	// TODO check proper pointer usage
	// TODO detect recursive templates (and panic :O)
	// add node <nodeName> to our graph
	// the new node inherits all dependencies of its parent
	// TODO add steps and potentially other types to isAbstract
	// add newNode to post set of all nodes in its pre set
	templateType := pc.templates[pc.nodes[nodeName].templateName].GetType()
	switch templateType {
	case v1alpha1.TemplateTypeDAG:
		pc.expandDAG(nodeName)
	case v1alpha1.TemplateTypeContainer:
		// nothing to expand
	default:
		panic("error templatetype not yet supported")
	}
}

func (pc *parseCtx) getDAGCtx(nodeName string) *dagContext {
	template := pc.templates[pc.nodes[nodeName].templateName]
	d := &dagContext{
		boundaryName: nodeName,
		boundaryID:   pc.woc.wf.NodeID(nodeName),
		tasks:        template.DAG.Tasks,
		visited:      make(map[string]bool),
		tmpl:         template,
		wf:           pc.woc.wf,
		// tmplCtx:        nil,
		// onExitTemplate: false,
		dependencies: make(map[string][]string),
		dependsLogic: make(map[string]string),
	}
	return d
}

func (pc *parseCtx) expandDAG(nodeName string) {
	nodeToExpand := pc.nodes[nodeName]
	tasks := pc.templates[nodeToExpand.templateName].DAG.Tasks
	dagCtx := pc.getDAGCtx(nodeName)
	// initialize new nodes
	newNodes := make(map[string]*parseNode)
	for _, task := range tasks {
		node := pc.newNode(dagCtx.taskNodeName(task.Name), task.Template)
		newNodes[node.name] = node
	}
	// add dependencies from dag
	for _, task := range tasks {
		// assuming that if A depends on B GetTaskDependencies(A) returns [B]
		dependencies := dagCtx.GetTaskDependencies(task.Name)
		for _, dep := range dependencies {
			fmt.Println(dep)
			fmt.Println(newNodes)
			pre := dagCtx.taskNodeName(dep)
			post := dagCtx.taskNodeName(task.Name)
			newNodes[pre].post[post] = true
			newNodes[post].pre[pre] = true
		}
	}
	// new nodes with no pre or post dependencies inherit the dependencies
	// of the node to expand
	// TODO check pre[x] = true for all x
	for _, newNode := range newNodes {
		if len(newNode.pre) == 0 {
			for dep := range nodeToExpand.pre {
				newNode.pre[dep] = true
				pc.nodes[dep].post[newNode.name] = true
			}
		}
		if len(newNode.post) == 0 {
			for dep := range nodeToExpand.post {
				newNode.post[dep] = true
				pc.nodes[dep].pre[newNode.name] = true
			}
		}
	}
	// add new nodes
	for newNodeName, newNode := range newNodes {
		pc.nodes[newNodeName] = newNode
	}
	pc.removeNode(nodeToExpand.name)
	// parse tasks
	for newNodeName := range newNodes {
		pc.expandNode(newNodeName)
	}
}

func (pc *parseCtx) removeNode(nodeName string) {
	delete(pc.nodes, nodeName)
	for _, node := range pc.nodes {
		delete(node.pre, nodeName)
		delete(node.post, nodeName)

	}
}

func (pc *parseCtx) printState() {
	for _, node := range pc.nodes {
		pc.woc.cwslog(node.name)
	}
}
