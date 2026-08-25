with open("internal/domain/circle/repository_pg.go", "r") as f:
    content = f.read()

content = content.replace("ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)", "ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)\n\tSelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error")

with open("internal/domain/circle/repository_pg.go", "w") as f:
    f.write(content)
