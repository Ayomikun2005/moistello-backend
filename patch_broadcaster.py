with open("internal/domain/circle/service.go", "r") as f:
    content = f.read()

content = content.replace("ContributionRecorded(ctx context.Context, circleID, userID string, roundNumber int, amount float64)", "ContributionRecorded(ctx context.Context, circleID, userID string, roundNumber int, amount float64)\n\tMemberPenalized(ctx context.Context, circleID, userID string, roundNumber int, penaltyAmount float64)")
content = content.replace("// Let's assume a generic penalize broadcast isn't added, but we've successfully stored the penalty.", "s.broadcaster.MemberPenalized(ctx, circleID, member.UserID.String(), roundNumber, penaltyAmt)")

with open("internal/domain/circle/service.go", "w") as f:
    f.write(content)
