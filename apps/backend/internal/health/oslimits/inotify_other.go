//go:build !linux

package oslimits

import "context"

// InotifyProbe is a no-op on non-Linux platforms.
type InotifyProbe struct{}

// NewInotifyProbe returns a stub probe that always reports Supported=false.
func NewInotifyProbe() *InotifyProbe {
	return &InotifyProbe{}
}

// Name returns the probe name.
func (p *InotifyProbe) Name() string { return "Inotify" }

// Category returns the probe category.
func (p *InotifyProbe) Category() string { return categoryID }

// Samples returns unsupported samples on non-Linux platforms.
func (p *InotifyProbe) Samples(_ context.Context) ([]Sample, error) {
	return []Sample{
		{ID: sampleIDInotifyInstances, Name: "Inotify instances", Unit: unitInstances, Supported: false},
		{ID: sampleIDInotifyWatches, Name: "Inotify watches", Unit: unitWatches, Supported: false},
	}, nil
}
