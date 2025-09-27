#!/bin/bash

# kill_pod.sh
# A simple chaos testing script that randomly deletes one of the emqx-go pods
# in the specified namespace to test the cluster's self-healing capabilities.

# --- Configuration ---
NAMESPACE="default"
APP_LABEL="app=emqx-go"

echo "Starting chaos test: randomly deleting an emqx-go pod."

# --- Get Pods ---
# Get a list of all pods matching the label, and store their names.
PODS=($(kubectl get pods -n ${NAMESPACE} -l ${APP_LABEL} -o jsonpath='{.items[*].metadata.name}'))

# Check if any pods were found.
if [ ${#PODS[@]} -eq 0 ]; then
  echo "No pods found with label '${APP_LABEL}' in namespace '${NAMESPACE}'."
  exit 1
fi

# --- Select a Random Pod ---
NUM_PODS=${#PODS[@]}
RANDOM_INDEX=$(( RANDOM % NUM_PODS ))
POD_TO_DELETE=${PODS[$RANDOM_INDEX]}

echo "Found ${NUM_PODS} pods. Randomly selected '${POD_TO_DELETE}' for termination."

# --- Delete the Pod ---
kubectl delete pod -n ${NAMESPACE} ${POD_TO_DELETE}

# --- Verification ---
# Check if the pod was deleted and is now being recreated.
echo "Pod '${POD_TO_DELETE}' has been deleted. The StatefulSet should be recreating it."
echo "You can verify this by running: kubectl get pods -n ${NAMESPACE} -w"

echo "Chaos test complete."