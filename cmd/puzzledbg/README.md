Usage:

```bash
curl -s -H "Origin: portal.privatecaptcha.com" https://api.privatecaptcha.com/puzzle?sitekey=abcdef | go run cmd/puzzledbg/main.go
```

For stub puzzle:

```bash
curl -s https://api.privatecaptcha.com/puzzle?sitekey=aaaaaaaabbbbccccddddeeeeeeeeeeee | go run cmd/puzzledbg/main.go -solve
```
