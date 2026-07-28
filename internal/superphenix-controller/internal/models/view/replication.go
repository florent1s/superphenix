package view

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ReplicationView struct {
	Name               string                `json:"name"`
	State              string                `json:"state,omitempty"`
	Message            string                `json:"message,omitempty"`
	LastSyncTime       *metav1.Time          `json:"lastSyncTime,omitempty"`
	LastSyncDuration   string                `json:"lastSyncDuration,omitempty"`
	LastSyncBytes      int64                 `json:"lastSyncBytes,omitempty"`
	LastCompletionTime *metav1.Time          `json:"lastCompletionTime,omitempty"`
	Class              *ReplicationClassView `json:"class,omitempty"`
}

type ReplicationClassView struct {
	Name                string `json:"name,omitempty"`
	Provisioner         string `json:"provisioner,omitempty"`
	SchedulingInterval  string `json:"schedulingInterval,omitempty"`
	SchedulingStartTime string `json:"schedulingStartTime,omitempty"`
	MirroringMode       string `json:"mirroringMode,omitempty"`
}
