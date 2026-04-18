// SPDX-License-Identifier: GPL-3.0-or-later
package domain

import "errors"

var (
	// ErrNotFound is returned when an entity does not exist.
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists is returned when creating a duplicate entity.
	ErrAlreadyExists = errors.New("already exists")

	// ErrHasDependencies is returned when deleting an entity that has
	// dependent entities (e.g., deleting a team with agents).
	ErrHasDependencies = errors.New("has dependencies")

	// ErrDepthExceeded is returned when a role inheritance chain would
	// exceed the configured max depth.
	ErrDepthExceeded = errors.New("role depth exceeded")

	// ErrCycleDetected is returned when a role parent assignment would
	// create a cycle in the inheritance chain.
	ErrCycleDetected = errors.New("role inheritance cycle detected")

	// ErrPermissionDenied is returned when an agent lacks a required permission.
	ErrPermissionDenied = errors.New("permission denied")

	// ErrPoolExhausted is returned when no free agent matches a claim.
	ErrPoolExhausted = errors.New("no free agents available")

	// ErrWorkflowMismatch is returned when a release or renew is issued by a
	// different workflow than the one currently holding the claim.
	ErrWorkflowMismatch = errors.New("workflow id does not match current claim")
)
