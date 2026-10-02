# ADR 003: Local browser workspace

Status: accepted for issue 26, based on main bc47d80.

Use embedded HTML, CSS and native ES modules served by Go. The implementation spike exercised project discovery, a real service-backed
preview and keyboard navigation in Chromium/Edge.
Use semantic controls, a small explicit state model and no client rendering engine.
This keeps the shipped application a single Go binary without a JavaScript build
step. Browser automation uses Playwright as a development-only dependency. Revisit
framework choice if the spike cannot express the editor clearly.

A typed HTTP boundary calls app services directly. Server-authorized project IDs
hide filesystem and database handles from the browser. A machine-local registry
stores file registrations separately from story data; PostgreSQL discovery uses
host configuration. Credentials and renderer profiles remain on the server.

Default serving is loopback. Validate Host and Origin, use a session CSRF token for
mutations, prohibit framing, bound JSON/uploads and validate all project references
before service reads. Artifact URLs are opaque authorized IDs, never arbitrary
filesystem URLs. Only PNG artwork and server-created preview/export artifacts are
exposed. Rendering remains in Go; preview never queues generation.

Generation has separate prepare/quote, explicit run, and select operations. The UI
shows source-first ordered references, provider attribution, license uncertainty,
job progress and the limits of cancellation. Polling replaces push connections.

Composition edits and server-owned undo/redo apply expected revisions through
app.Edit; external edits invalidate the history head and must surface conflicts.
Undo never restores raw files or bypasses locks. Approval/lock transitions remain
explicit actions rather than implicit undo side effects.

Delivery maps project discovery/navigation/build to #13; composition, lettering,
review and locks map to #14. Advanced masks remain in #14. Neither overlapping
issue is automatically closed by this implementation.
