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
)
