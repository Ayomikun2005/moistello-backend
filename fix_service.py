with open("internal/domain/circle/service.go", "r") as f:
    content = f.read()

content = content.replace("RemoveMember(ctx context.Context, circleID, callerID, memberAddress string, reason string) error\n}", "RemoveMember(ctx context.Context, circleID, callerID, memberAddress string, reason string) error\n\tProcessMissedContributions(ctx context.Context, circleID string, roundNumber int) error\n}")

with open("internal/domain/circle/service.go", "w") as f:
    f.write(content)

with open("internal/domain/circle/service_test.go", "r") as f:
    test_content = f.read()

test_content = test_content.replace("repo := new(mocks.Repository)", "repo := new(circleMocks.Repository)")

with open("internal/domain/circle/service_test.go", "w") as f:
    f.write(test_content)
