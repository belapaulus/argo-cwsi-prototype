#!/bin/sh
set -eu

kubectl delete serviceaccount cwsaccount
kubectl delete clusterrole cwsrole
kubectl delete clusterrolebinding cwsbinding
kubectl delete pod workflow-scheduler
#kubectl delete service workflow-scheduler
pids=$(lsof -t -s TCP:LISTEN -i ":8081" || true)

if [ "$pids" != "" ]; then
  kill $pids
fi

CWSDIR=cws
kubectl apply -f $CWSDIR/serviceaccount.yaml
kubectl apply -f $CWSDIR/clusterrole.yaml
kubectl apply -f $CWSDIR/clusterrolebinding.yaml
kubectl apply -f $CWSDIR/cws.yaml
#kubectl apply -f $CWSDIR/service.yaml
#
kubectl wait --for=condition=ready --timeout=-1s pod workflow-scheduler
kubectl port-forward workflow-scheduler 8081:8080 &
