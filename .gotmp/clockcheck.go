package main
import (
  "fmt"
  "os"
  "github.com/redraskal/r6-dissect/dissect"
)
func main(){
  f, err := os.Open("replays/360_pattern.rec")
  if err != nil { panic(err) }
  defer f.Close()
  r, err := dissect.NewReader(f)
  if err != nil { panic(err) }
  if _, err := r.Write(io.Discard); err != nil { _ = err }
  _ = r
  fmt.Println("todo")
}
