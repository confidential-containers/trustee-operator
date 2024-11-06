#!/bin/bash

kubectl create secret generic kbsres1 --from-literal key1=res1val1 --from-literal key2=res1val2 -n trustee-operator-system

