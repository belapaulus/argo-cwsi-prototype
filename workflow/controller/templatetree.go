package controller

import (
	"fmt"
	"hash/fnv"

	"github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
)

// TODO move file to a better suited package

type parseNode struct {
	// set of nodes that this node depends on
	pre stringset
	// set of nodes that depend on this node
	post         stringset
	name         string
	templateName string
	// unique identifier for the scheduler
	id       uint32
	nodeType string
}

type graph map[string]*parseNode

type parseCtx struct {
	// map to look up templates by name
	templates map[string]*v1alpha1.Template
	seen      stringset
	// map that contains all nodes by name
	nodes graph
	// node counter currently used to create id
	woc *wfOperationCtx
}

func (pc *parseCtx) printGraph() {
	pc.woc.cwslog("--printing graph--")
	for nodeName, node := range pc.nodes {
		for edge := range node.pre {
			pc.woc.cwslog(edge + " -> " + nodeName)
		}
	}
}

func getNodeIDFromName(name string) uint32 {
	// TODO
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	//return int(h.Sum32()) / 1000000
	return h.Sum32()
}

func (woc *wfOperationCtx) parseTemplateTree() graph {
	// create parse context
	pc := &parseCtx{
		templates: make(map[string]*v1alpha1.Template),
		nodes:     make(graph),
		seen:      make(stringset),
		woc:       woc,
	}
	// store all workflow templates in map
	// TODO why not &template?
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
		pre:          make(stringset),
		post:         make(stringset),
		name:         nodeName,
		templateName: templateName,
		id:           getNodeIDFromName(nodeName),
		nodeType:     "PROCESS",
	}
}

func (pc *parseCtx) expandNode(nodeName string) {
	// TODO document set usage (no set[key] = false)
	// TODO check proper pointer usage
	// TODO detect recursive templates (and panic :O)
	node := pc.nodes[nodeName]
	template := node.templateName
	if pc.seen.contains(template) {
		node.nodeType = "RECURSE"
		return
	}
	pc.seen.add(template)
	defer pc.seen.del(template)
	templateType := pc.templates[template].GetType()
	pc.woc.cwslog("graph before expanding " + nodeName + " with template " + template + " of type " + string(templateType))
	pc.printGraph()
	switch templateType {
	case v1alpha1.TemplateTypeContainer:
		// - this is a leaf template
	case v1alpha1.TemplateTypeContainerSet:
		// - executes several containers in a single pod
		// - dependencies between containers can be specified but since
		//   a pod is the smallest unit the cws understands we treat a
		//   container set as an atomic entity
		// - this is a leaf template
	case v1alpha1.TemplateTypeSteps:
		pc.expandSteps(nodeName)
	case v1alpha1.TemplateTypeDAG:
		pc.expandDAG(nodeName)
	case v1alpha1.TemplateTypeScript:
		// - like container but with an additional option to pass a
		//   script
		// - this is a leaf template
	case v1alpha1.TemplateTypeResource:
		// - gives the user a way to manipulate kubernetes resources
		//   from within a workflow
		// - argo creates a container that starts "argoexec resource"
		// - this is a leaf template
	case v1alpha1.TemplateTypeData:
		// - gives the user a way to directly manipulate data sources
		// - argo creates a container that starts "argoexec data"
		// - this is a leaf node
	case v1alpha1.TemplateTypeSuspend:
		// - suspend this part of the workflow for a given time or
		// - until user input is received
		// - this is a leaf node BUT can not be scheduled
		pc.skipNode(nodeName)
	case v1alpha1.TemplateTypeHTTP:
		// TODO
	case v1alpha1.TemplateTypePlugin:
		// TODO
	case v1alpha1.TemplateTypeUnknown:
		// TODO
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
	newNodes := make(stringset)
	for _, task := range tasks {
		node := pc.newNode(dagCtx.taskNodeName(task.Name), task.Template)
		newNodes.add(node.name)
		pc.nodes[node.name] = node
	}
	// add dependencies from dag
	for _, task := range tasks {
		// assuming that if A depends on B GetTaskDependencies(A) returns [B]
		deps := dagCtx.GetTaskDependencies(task.Name)
		pre := make(stringset)
		for _, dep := range deps {
			pre.add(dagCtx.taskNodeName(dep))
		}
		post := stringSetFromString(dagCtx.taskNodeName(task.Name))
		pc.addAllEdges(pre, post)
	}
	// new nodes with no pre or post dependencies inherit the dependencies
	// of the node being expanded
	// TODO check pre[x] = true for all x
	for newNode := range newNodes {
		if len(pc.nodes[newNode].pre) == 0 {
			pc.addAllEdges(nodeToExpand.pre, stringSetFromString(newNode))
		}
		if len(pc.nodes[newNode].post) == 0 {
			pc.addAllEdges(stringSetFromString(newNode), nodeToExpand.post)
		}
	}
	// remove expanded node
	pc.removeNode(nodeToExpand.name)
	// parse tasks
	for newNodeName := range newNodes {
		pc.expandNode(newNodeName)
	}
}

func (pc *parseCtx) expandSteps(nodeName string) {
	nodeToExpand := pc.nodes[nodeName]
	previous := nodeToExpand.pre
	template := pc.templates[nodeToExpand.templateName]
	newNodes := make(stringset)
	for i, stepGroup := range template.Steps {
		current := make(stringset)
		stepGroupName := fmt.Sprintf("%s[%d]", nodeName, i)
		for _, step := range stepGroup.Steps {
			stepName := fmt.Sprintf("%s.%s", stepGroupName, step.Name)
			node := pc.newNode(stepName, step.Template)
			newNodes.add(node.name)
			current.add(node.name)
			pc.nodes[node.name] = node
		}
		pc.addAllEdges(previous, current)
		previous = current
	}
	pc.addAllEdges(previous, nodeToExpand.post)
	pc.removeNode(nodeToExpand.name)
	for newNodeName := range newNodes {
		pc.expandNode(newNodeName)
	}
}

func (pc *parseCtx) addAllEdges(pre stringset, post stringset) {
	// adds all possible edges between the nodes in pre and post
	// assumes that the node in pre and post exist in pc.nodes
	for node := range pre {
		pc.nodes[node].post.addAll(post)
	}
	for node := range post {
		pc.nodes[node].pre.addAll(pre)
	}
}

func (pc *parseCtx) removeNode(nodeName string) {
	// removes a node and all its edges from the graph
	delete(pc.nodes, nodeName)
	for _, node := range pc.nodes {
		delete(node.pre, nodeName)
		delete(node.post, nodeName)

	}
}

func (pc *parseCtx) skipNode(nodeName string) {
	// removes a node from the graph but "keeps" the edges. after the
	// operation, all nodes that depended on the given node will depend on
	// all the nodes that the given node depended on itself.
	node := pc.nodes[nodeName]
	for pre := range node.pre {
		for post := range node.post {
			pc.nodes[pre].post[post] = true
			pc.nodes[post].pre[pre] = true
		}
	}
	pc.removeNode(nodeName)
}
