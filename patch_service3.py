import re

with open("internal/domain/circle/service.go", "r") as f:
    content = f.read()

content = content.replace("apperrors.NewInvalidInputError(\"invalid circle ID\")", "apperrors.ErrInvalidInput")
content = content.replace("apperrors.NewNotFoundError(\"circle not found\")", "apperrors.ErrNotFound")

with open("internal/domain/circle/service.go", "w") as f:
    f.write(content)
