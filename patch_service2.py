with open("internal/domain/circle/service.go", "r") as f:
    content = f.read()

content = content.replace(
    "s.broadcaster.MemberPenalized(ctx, circleID, member.UserID.String(), roundNumber, penaltyAmt)",
    "if s.broadcaster != nil {\n\t\t\t\ts.broadcaster.MemberPenalized(ctx, circleID, member.UserID.String(), roundNumber, penaltyAmt)\n\t\t\t}"
)

with open("internal/domain/circle/service.go", "w") as f:
    f.write(content)
