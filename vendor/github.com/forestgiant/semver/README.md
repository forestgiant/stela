## Install
`go get -u github.com/forestgiant/semver`

## Usage
```
import "github.com/forestgiant/semver"

// Set Semantic Version
err = semver.SetVersion("1.0.0")
if err != nil {
  log.Fatal(err)
}
```
