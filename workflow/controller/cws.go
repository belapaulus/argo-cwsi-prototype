package controller

import (
	"encoding/json"
	"github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"hash/fnv"
	"net/http"
	"strconv"
	"strings"
)

type registerWorkflowRequestBody struct {
	//VolumeClaims []volumeClaims `json:"volumeClaims"`
	//WorkDir string `json:"workDir"`
	Dns          string `json:"dns"`
	TraceEnabled bool   `json:"traceEnabled"`
	Namespace    string `json:"namespace"`
	//CostFunction string `json:"costFunction"`
	Strategy string `json:"strategy"`
	//MaxCopyTaskPerNode int `json:"maxCopyTaskPerNode"`
	//MaxWaitingCopyTaskPerNode int `json:"maxWaitingCopyTaskPerNode"`
}

type vertex struct {
	Label string `json:"label"`
	Uid   uint32 `json:"uid"`
	//Rank  int    `json:"rank"`
	Type string `json:"type"`
}

type edge struct {
	Uid   int    `json:"uid"`
	Label string `json:"label"`
	From  uint32 `json:"from"`
	To    uint32 `json:"to"`
}

type taskParams struct{}
type taskInputs struct{}

type task struct {
	Task            string     `json:"task"`
	Name            string     `json:"name"`
	SchedulerParams taskParams `json:"schedulerParams"`
	Inputs          taskInputs `json:"inputs"`
	RunName         string     `json:"runName"`
	Cpus            int        `json:"cpus"`
	MemoryInBytes   int        `json:"memoryInBytes"`
	WorkDir         string     `json:"workDir"`
}

// TODO: localhost:8081 only works if the controller runs
// outside the cluster and we enable port forwarding for the cws
// if the controller runs as a pod in the cluster we would use
// "http://workflow-scheduler" as the url. Ideally there is one
// solution for both cases

// woc.submitTemplate(woc.execWf.Spec.Entrypoint, woc.execWf.ObjectMeta.Name, nil, nil) {

// TODO: close response bodies if necessary

// var cwsURL = "http://localhost:8081"

var cwsURL = "http://workflow-scheduler"
var registeredTasks = 0

func (woc *wfOperationCtx) cws() {
	//deleteWF()
	if !woc.execWf.Status.RegisteredWithCWS {
		woc.registerWF()
		woc.submitWF()
		woc.wf.Status.CWSRegisteredTasks = make(map[string]bool)
		woc.wf.Status.RegisteredWithCWS = true
	}
}

func (woc *wfOperationCtx) cwslog(msg string) {
	woc.log.Info("cws: " + msg)
}

func (woc *wfOperationCtx) cwsPOST(path string, body any) *http.Response {
	jsonBytes, err := json.Marshal(body)
	if err != nil {
		panic(err.Error())
	}
	jsonString := string(jsonBytes)
	woc.cwslog("..." + jsonString)
	resp, err := http.Post(cwsURL+"/v1/scheduler/"+woc.execWf.ObjectMeta.Name+path, "application/json", strings.NewReader(jsonString))
	if err != nil {
		panic(err.Error())
	}
	return resp
}

func (woc *wfOperationCtx) registerWF() {
	woc.cwslog("registering workflow")
	body := registerWorkflowRequestBody{
		Dns:          "",
		TraceEnabled: true,
		Namespace:    "argo",
		Strategy:     woc.execWf.Spec.SchedulerName,
	}
	resp := woc.cwsPOST("", body)
	// TODO: read response code and handle errors
	if resp.StatusCode != 200 {
		panic(resp.Status)
	}
	woc.cwslog("...ok")
}

func (woc *wfOperationCtx) deleteWF() {
	woc.cwslog("deleting workflow")
	client := &http.Client{}
	req, err := http.NewRequest("DELETE", cwsURL+"/v1/scheduler/"+woc.execWf.ObjectMeta.Name, nil)
	if err != nil {
		panic(err.Error())
	}
	resp, err := client.Do(req)
	if err != nil {
		panic(err.Error())
	}
	if resp.StatusCode != 200 {
		panic("..." + resp.Status)
	}
	woc.cwslog("...ok")
}

func (woc *wfOperationCtx) submitWF() {
	// submit graph
	// TODO save normalization map
	nodes := woc.parseTemplateTree()
	woc.wf.Status.CWSNodes = make(map[string]bool)
	for node := range nodes {
		woc.cwslog("node: " + node)
		woc.wf.Status.CWSNodes[node] = true
	}
	edgeCount := 0
	var verticesBody []vertex
	var edgesBody []edge
	var seen map[uint32]bool = make(map[uint32]bool)
	for _, node := range nodes {
		// TODO Rank
		if seen[node.id] {
			panic(node.id)
		} else {
			seen[node.id] = true
		}
		verticesBody = append(verticesBody, vertex{
			Label: node.name,
			Uid:   node.id,
			//Rank:  0,
			Type: node.nodeType,
		})
		// TODO Label
		for e := range node.post {
			edgesBody = append(edgesBody, edge{
				Label: "",
				Uid:   edgeCount,
				From:  node.id,
				To:    nodes[e].id,
			})
			edgeCount += 1
		}
	}
	resp := woc.cwsPOST("/DAG/vertices", verticesBody)
	if resp.StatusCode != 200 {
		panic("could not submit vertices: " + resp.Status)
	}
	if edgesBody == nil {
		return
	}
	resp = woc.cwsPOST("/DAG/edges", edgesBody)
	if resp.StatusCode != 200 {
		panic("could not submit edges" + resp.Status)
	}
}

func (woc *wfOperationCtx) startBatch() {
	woc.cwslog("start batch")
	client := &http.Client{}
	req, err := http.NewRequest("PUT", cwsURL+"/v1/scheduler/"+woc.execWf.ObjectMeta.Name+"/startBatch", nil)
	resp, err := client.Do(req)
	if err != nil {
		panic(err.Error())
	}
	// TODO: read response code and handle errors
	if resp.StatusCode != 200 {
		panic("..." + resp.Status)
	}
	woc.cwslog("...ok")
}

func (woc *wfOperationCtx) endBatch() {
	woc.cwslog("end batch")
	client := &http.Client{}
	req, err := http.NewRequest("PUT", cwsURL+"/v1/scheduler/"+woc.execWf.ObjectMeta.Name+"/endBatch", strings.NewReader(strconv.Itoa(registeredTasks)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		panic(err.Error())
	}
	// TODO: read response code and handle errors
	if resp.StatusCode != 200 {
		panic("..." + resp.Status)
	}
	registeredTasks = 0
	woc.cwslog("...ok")
}

func (woc *wfOperationCtx) registerTask(node *v1alpha1.NodeStatus, podName string) {
	name := woc.normalizeTaskName(node.Name)
	woc.cwslog("registering task '" + node.Name + "' with normalized name '" + name + "'")
	// TODO: understand task fields
	body := task{
		Task:            name,
		Name:            name,
		SchedulerParams: taskParams{},
		Inputs:          taskInputs{},
		RunName:         podName,
		Cpus:            1,
		MemoryInBytes:   128000000,
		WorkDir:         "/",
	}
	h := fnv.New32a()
	// using podname because id must be unique
	_, _ = h.Write([]byte(podName))
	taskID := h.Sum32()
	resp := woc.cwsPOST("/task/"+strconv.Itoa(int(taskID)), body)
	// TODO: read response code and handle errors
	if resp.StatusCode != 200 {
		panic("...error")
	}
	registeredTasks++
	woc.cwslog("...ok")
}

func (woc *wfOperationCtx) normalizeTaskName(taskName string) string {
	for i := range taskName {
		prefix := taskName[0 : i+1]
		if woc.wf.Status.CWSNodes[prefix] {
			woc.cwslog("normalized taskName '" + taskName + "' to '" + prefix + "'")
			return prefix
		}
		woc.cwslog(prefix + " not in nodes")
	}
	panic("unknown cws node " + taskName)
}
