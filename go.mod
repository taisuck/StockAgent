module github.com/taisuck/StockAgent

// 코드가 실제로 요구하는 최소 버전만 명시한다. 패치 버전까지 박아두면
// 다른 개발/배포 머신에서 툴체인을 강제로 내려받게 된다.
go 1.25.0

require (
	go.mongodb.org/mongo-driver/v2 v2.9.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/klauspost/compress v1.19.2 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.2.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/text v0.39.0 // indirect
)
