import re

with open("internal/api/handler/swap_handler.go", "r") as f:
    content = f.read()

content = re.sub(r'c\.JSON\(http\.StatusUnauthorized,\s*response\.Error\("([^"]+)"\)\)', r'response.Error(c, http.StatusUnauthorized, "\1", nil)', content)
content = re.sub(r'c\.JSON\(http\.StatusBadRequest,\s*response\.Error\("([^"]+)" \+ err\.Error\(\)\)\)', r'response.Error(c, http.StatusBadRequest, "\1", err)', content)
content = re.sub(r'c\.JSON\(http\.StatusInternalServerError,\s*response\.Error\(([^)]+)\)\)', r'response.Error(c, http.StatusInternalServerError, "internal error", \1)', content)

with open("internal/api/handler/swap_handler.go", "w") as f:
    f.write(content)
