# Testing Overview

Muto maintains high code quality through comprehensive automated testing at multiple levels.

## Testing Strategy

Muto uses a three-tier testing pyramid:

```
         ▲
        /│\
       / │ \    E2E Tests
      /  │  \   - Full system
     /   │   \  - Slow (min)
    ─────────── 
   /     │     \  Integration Tests
  /      │      \ - Components + platform
 /       │       \- Medium (s)
─────────────────
│       │       │  Unit Tests
│ Fast (ms)    │  - Single function
│  Isolated    │  - Deterministic
├───────────────┤
```

## Test Levels

### Unit Tests (80% of tests)

Fast, focused tests for individual functions and types.

**Characteristics:**
- No external dependencies (mock them)
- Run in milliseconds
- Highly deterministic
- Easy to debug

**Example:**
```bash
go test ./core/agent -v
```

**Location:** `*_test.go` next to source code

### Integration Tests (15% of tests)

Tests for multiple components working together with real platforms.

**Characteristics:**
- Use real Kubernetes or CloudFoundry
- Take seconds to minutes
- Verify component communication
- Catch integration issues

**Example:**
```bash
make test-integration-k8s
```

**Location:** `test/integration/`

### End-to-End Tests (5% of tests)

Full system tests simulating complete workflows.

**Characteristics:**
- Full deployment and operation
- Take 10-20 minutes
- Run on real platforms
- Comprehensive coverage

**Example:**
```bash
make test-e2e
```

**Location:** `test/e2e/`

## Running Tests Locally

### Quick Tests (5 min)

```bash
# Unit tests only
make test-unit
```

### Full Test Suite (30 min)

```bash
# Unit + integration
make test
```

### Platform-Specific

```bash
# Kubernetes only
make test-k8s

# CloudFoundry only
make test-cf
```

## Test Coverage

Target coverage by component:

| Component | Target | Current |
|-----------|--------|---------|
| core/scheduler | 85% | 84% |
| core/agent | 90% | 88% |
| platform/k8s | 75% | 72% |
| platform/cf | 75% | 70% |
| mcp/tools | 80% | 78% |

View coverage report:

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## CI/CD Pipeline

Tests run automatically on every push and PR:

```
Push → GitHub Actions
  ├─ Lint (2 min)
  ├─ Build (3 min)
  ├─ Unit Tests (5 min)
  ├─ Integration Tests (15 min)
  └─ E2E Tests (20 min)
  
✓ All pass → Merge allowed
✗ Any fail → PR blocked
```

## Testing Best Practices

### 1. Test One Thing Per Test

```go
// ✅ Good
It("rejects job with missing tenant", func() {
    // Single assertion
})

// ❌ Bad
It("validates job spec", func() {
    // Tests multiple things
})
```

### 2. Use Meaningful Names

```go
// ✅ Good: Describes behavior
It("transitions from Pending to Scheduled", func() { ... })

// ❌ Bad: Vague
It("test state", func() { ... })
```

### 3. Test Behavior, Not Implementation

```go
// ✅ Good: Tests behavior
Expect(job.Status).To(Equal(Completed))

// ❌ Bad: Tests internals
Expect(job.internalState).To(Equal(done))
```

### 4. Mock External Dependencies

```go
// ✅ Good: Mock platform
mockPlatform.On("CreateTask").Return(nil)

// ❌ Bad: Use real platform in unit test
env.CreateTask() // Slow, flaky, unreliable
```

### 5. Clean Up After Tests

```go
// ✅ Good: Always cleanup
AfterEach(func() {
    env.Teardown(ctx)
})

// ❌ Bad: Leave resources around
// Tests interfere with each other
```

## Writing Tests

### Test Structure

Most tests follow this pattern:

```go
var _ = Describe("Component", Label("unit"), func() {
    var obj *ObjectUnderTest
    
    BeforeEach(func() {
        obj = NewObject()
    })
    
    It("does something", func() {
        // Arrange: set up test data
        input := "test"
        
        // Act: execute
        result := obj.Method(input)
        
        // Assert: verify
        Expect(result).To(Equal("expected"))
    })
})
```

### Table-Driven Tests

For testing multiple inputs:

```go
DescribeTable("Validate",
    func(input string, expectErr bool) {
        err := Validate(input)
        if expectErr {
            Expect(err).To(HaveOccurred())
        } else {
            Expect(err).NotTo(HaveOccurred())
        }
    },
    Entry("valid input", "valid", false),
    Entry("empty input", "", true),
    Entry("special chars", "!@#$", true),
)
```

## Troubleshooting Tests

### Tests Won't Run

```bash
# Ensure Go is installed
go version

# Ensure dependencies are downloaded
go mod download

# Run with verbose output
go test ./... -v
```

### Tests Timeout

```bash
# Increase timeout
go test ./... -timeout 40m

# Run single test with debug
go test -run TestName -v -ginkgo.show-node-events=all
```

### Tests Fail Intermittently

**Flaky tests usually indicate:**
- Missing explicit waits (use `Eventually()`)
- Race conditions (run with `-race`)
- Non-deterministic test data

```bash
go test -race ./...
```

### Can't Create Test Cluster

```bash
# Check Docker
docker ps

# Check Kind
kind cluster-info

# Recreate cluster
kind delete cluster --name test
kind create cluster --name test
```

## Performance

### Optimize Unit Tests

- Minimize setup/teardown
- Use mocks instead of real objects
- Avoid sleep, use explicit waits

### Optimize Integration Tests

- Reuse test environment across tests when possible
- Run tests in parallel: `go test -parallel 8 ./...`
- Cache test images

## Continuous Integration

Tests run in GitHub Actions. View results:

1. **On PR:** Check "Checks" tab
2. **On main:** Check GitHub Actions history
3. **Local:** Run `make test-all`

All tests must pass before merging to main.

---

## Next Steps

- **[Unit Tests](./unit-tests.md)** — Writing unit tests
- **[Integration Tests](./integration-tests.md)** — Integration test guide
- **[E2E Tests](./e2e-tests.md)** — End-to-end tests
- **[Running Tests Locally](./running-locally.md)** — Local testing setup

---

**Last Updated:** 2026-09-06
