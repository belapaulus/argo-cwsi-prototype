package controller

import (
	"encoding/json"
	"errors"
	"github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
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
	Rank  int    `json:"rank"`
	Type  string `json:"type"`
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

var cwsURL = "http://localhost:8081"
var registeredTasks = 0

func (woc *wfOperationCtx) cws() {
	//deleteWF()
	if !woc.execWf.Status.RegisteredWithCWS {
		if woc.registerWF() && woc.submitWF() {
			woc.wf.Status.RegisteredWithCWS = true
		} else {
			panic("could not register and submit workflow")
		}
	}
}

func (woc *wfOperationCtx) cwslog(msg string) {
	woc.log.Info("cws: " + msg)
}

func (woc *wfOperationCtx) cwsPOST(path string, body any) (*http.Response, error) {
	jsonBytes, err := json.Marshal(body)
	if err != nil {
		woc.cwslog(err.Error())
		return nil, errors.New("can not marshal body")
	}
	jsonString := string(jsonBytes)
	woc.cwslog("..." + jsonString)
	resp, err := http.Post(cwsURL+"/v1/scheduler/"+woc.execWf.ObjectMeta.Name+path, "application/json", strings.NewReader(jsonString))
	if err != nil {
		woc.cwslog(err.Error())
		return nil, errors.New("error making request")
	}
	return resp, nil
}

func (woc *wfOperationCtx) registerWF() bool {
	woc.cwslog("registering workflow")
	body := registerWorkflowRequestBody{
		Dns:          "",
		TraceEnabled: true,
		Namespace:    "argo",
		Strategy:     "fifo-fair",
	}
	resp, err := woc.cwsPOST("", body)
	// TODO: read response code and handle errors
	if err != nil || resp.StatusCode != 200 {
		woc.cwslog("...error")
		return false
	}
	woc.cwslog("...ok")
	return true
}

func (woc *wfOperationCtx) deleteWF() bool {
	woc.cwslog("deleting workflow")
	client := &http.Client{}
	req, err := http.NewRequest("DELETE", cwsURL+"/v1/scheduler/"+woc.execWf.ObjectMeta.Name, nil)
	if err != nil {
		woc.cwslog(err.Error())
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		woc.cwslog(err.Error())
		return false
	}
	if resp.StatusCode != 200 {
		woc.cwslog("..." + resp.Status)
		return false
	}
	woc.cwslog("...ok")
	return true
}

func (woc *wfOperationCtx) submitWF() bool {
	// submit graph
	nodes := woc.parseTemplateTree()
	edgeCount := 0
	var verticesBody []vertex
	var edgesBody []edge
	for _, node := range nodes {
		// TODO Rank
		verticesBody = append(verticesBody, vertex{
			Label: node.name,
			Uid:   node.id,
			Rank:  0,
			Type:  "PROCESS",
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
	resp, err := woc.cwsPOST("/DAG/vertices", verticesBody)
	if err != nil {
		panic("could not submit vertices: " + err.Error())
	}
	if resp.StatusCode != 200 {
		panic("could not submit vertices: " + resp.Status)
	}
	if edgesBody == nil {
		return true
	}
	resp, err = woc.cwsPOST("/DAG/edges", edgesBody)
	if err != nil {
		panic("could not submit edges" + err.Error())
	}
	if resp.StatusCode != 200 {
		panic("could not submit edges" + resp.Status)
	}
	return true
}

func (woc *wfOperationCtx) startBatch() bool {
	woc.cwslog("start batch")
	client := &http.Client{}
	req, err := http.NewRequest("PUT", cwsURL+"/v1/scheduler/"+woc.execWf.ObjectMeta.Name+"/startBatch", nil)
	resp, err := client.Do(req)
	if err != nil {
		woc.cwslog(err.Error())
		return false
	}
	// TODO: read response code and handle errors
	if resp.StatusCode != 200 {
		woc.cwslog("..." + resp.Status)
		return false
	}
	woc.cwslog("...ok")
	return true
}

func (woc *wfOperationCtx) endBatch() bool {
	woc.cwslog("end batch")
	client := &http.Client{}
	req, err := http.NewRequest("PUT", cwsURL+"/v1/scheduler/"+woc.execWf.ObjectMeta.Name+"/endBatch", strings.NewReader(strconv.Itoa(registeredTasks)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		woc.cwslog(err.Error())
		return false
	}
	// TODO: read response code and handle errors
	if resp.StatusCode != 200 {
		woc.cwslog("..." + resp.Status)
		return false
	}
	registeredTasks = 0
	woc.cwslog("...ok")
	return true
}

func (woc *wfOperationCtx) registerTask(node *v1alpha1.NodeStatus, podName string) bool {
	woc.cwslog("registering task: " + node.Name)
	// TODO: understand task fields
	body := task{
		Task:            node.Name,
		Name:            node.Name,
		SchedulerParams: taskParams{},
		Inputs:          taskInputs{},
		RunName:         podName,
		Cpus:            1,
		MemoryInBytes:   128000000,
		WorkDir:         "/",
	}
	taskID := 0
	resp, err := woc.cwsPOST("/task/"+strconv.Itoa(taskID), body)
	// TODO: read response code and handle errors
	if err != nil || resp.StatusCode != 200 {
		woc.cwslog("...error")
		return false
	}
	registeredTasks++
	woc.cwslog("...ok")
	return true
}
