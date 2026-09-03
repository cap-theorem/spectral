# spectral

`spectral` is an experimental decentralized service-discovery system for high-churn environments.

Each sidecar participates in a sparse expander overlay, advertises local services, and discovers live providers without relying on a central registry. The design combines DEX-inspired graph maintenance with randomized discovery and epoch-based topology resizing.

## How it works

- Constant-degree expander overlay for scalable connectivity
- Randomized walks for membership sampling
- Parallel multi-path service lookups
- Stable node identities across topology epochs
- Join, graceful-leave, and crash handling
- Inflation and deflation as population changes

## Quick start

Requirements: Go 1.26+ and Docker.
