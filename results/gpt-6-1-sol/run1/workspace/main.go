package main

import (
 "crypto/rand"
 "crypto/subtle"
 "encoding/json"
 "errors"
 "flag"
 "fmt"
 "io"
 "log"
 "net/http"
 "net/url"
 "os"
 "path/filepath"
 "regexp"
 "sort"
 "strings"
 "sync"
 "time"
)

type Link struct {
 Code string `json:"code"`
 URL string `json:"url"`
 CreatedAt time.Time `json:"created_at"`
 Visits uint64 `json:"visits"`
}

type Store struct { mu sync.Mutex; path string; links map[string]Link }

func openStore(path string) (*Store,error) {
 s:=&Store{path:path,links:make(map[string]Link)}
 b,err:=os.ReadFile(path)
 if errors.Is(err,os.ErrNotExist) { return s,nil }; if err!=nil{return nil,err}
 var links []Link
 if err=json.Unmarshal(b,&links);err!=nil{return nil,fmt.Errorf("read storage: %w",err)}
 for _,l:=range links {
  if l.Code=="" || !validURL(l.URL) {return nil,fmt.Errorf("invalid stored link")}
  if _,ok:=s.links[l.Code];ok{return nil,fmt.Errorf("duplicate stored code")}
  s.links[l.Code]=l
 }
 return s,nil
}

func sortedLinks(m map[string]Link) []Link {
 out:=make([]Link,0,len(m));for _,l:=range m {out=append(out,l)}
 sort.Slice(out,func(i,j int)bool{if out[i].CreatedAt.Equal(out[j].CreatedAt){return out[i].Code<out[j].Code};return out[i].CreatedAt.Before(out[j].CreatedAt)})
 return out
}

// Commit via a synced temporary file and atomic rename. Never overwrite the live
// file in place. The mutex remains held through the durable commit.
func (s *Store) save(next map[string]Link) error {
 b,err:=json.Marshal(sortedLinks(next));if err!=nil{return err}
 dir:=filepath.Dir(s.path)
 f,err:=os.CreateTemp(dir,".shortener-*");if err!=nil{return err}
 name:=f.Name();defer os.Remove(name)
 if _,err=f.Write(b);err!=nil{f.Close();return err}
 if err=f.Sync();err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 if err=os.Rename(name,s.path);err!=nil{return err}
 // The rename has committed; keep memory consistent even if directory sync fails.
 s.links=next
 d,err:=os.Open(dir);if err!=nil{return err};defer d.Close()
 return d.Sync()
}
func (s *Store) copy() map[string]Link {m:=make(map[string]Link,len(s.links));for k,v:=range s.links{m[k]=v};return m}

var aliasPattern=regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
func validURL(raw string) bool {
 if len(raw)==0 || len(raw)>2048{return false}
 u,err:=url.Parse(raw)
 return err==nil && (u.Scheme=="http" || u.Scheme=="https") && u.Host!="" && u.Hostname()!="" && u.IsAbs()
}
func randomCode() (string,error) {
 const alphabet="ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
 out:=make([]byte,7)
 for i:=0;i<len(out); {var b [1]byte;if _,err:=rand.Read(b[:]);err!=nil{return "",err};if b[0]<248{out[i]=alphabet[int(b[0])%len(alphabet)];i++}}
 return string(out),nil
}

type Server struct { store *Store; token string }
func jsonResponse(w http.ResponseWriter,status int,v any) {w.Header().Set("Content-Type","application/json");w.WriteHeader(status);_ = json.NewEncoder(w).Encode(v)}
func fail(w http.ResponseWriter,status int,msg string) {jsonResponse(w,status,map[string]string{"error":msg})}
func (s *Server) auth(w http.ResponseWriter,r *http.Request) bool {
 if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")),[]byte("Bearer "+s.token))!=1 {fail(w,401,"unauthorized");return false};return true
}
func method(w http.ResponseWriter,allow string) {w.Header().Set("Allow",allow);fail(w,405,"method not allowed")}
func (s *Server) ServeHTTP(w http.ResponseWriter,r *http.Request) {
 p:=r.URL.Path
 if p=="/api/links" {
  switch r.Method {
  case http.MethodPost:s.create(w,r)
  case http.MethodGet:if s.auth(w,r){s.store.mu.Lock();links:=sortedLinks(s.store.links);s.store.mu.Unlock();jsonResponse(w,200,links)}
  default:method(w,"GET, POST")
  };return
 }
 if strings.HasPrefix(p,"/api/links/") && len(strings.TrimPrefix(p,"/api/links/"))>0 && !strings.Contains(strings.TrimPrefix(p,"/api/links/"),"/") {
  if r.Method!=http.MethodDelete {method(w,"DELETE");return}
  if !s.auth(w,r){return};s.remove(w,strings.TrimPrefix(p,"/api/links/"));return
 }
 if len(p)>1 && !strings.Contains(p[1:],"/") && p!="/api" {
  if r.Method!=http.MethodGet {method(w,"GET");return};s.follow(w,r,p[1:]);return
 }
 fail(w,404,"not found")
}
func (s *Server) create(w http.ResponseWriter,r *http.Request) {
 // Read the whole bounded body first so oversized bodies always return 413.
 b,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,1<<20))
 if err!=nil{var max *http.MaxBytesError;if errors.As(err,&max){fail(w,413,"request body too large")}else{fail(w,400,"invalid request body")};return}
 var req struct {URL string `json:"url"`;Alias *string `json:"alias"`}
 dec:=json.NewDecoder(strings.NewReader(string(b)));dec.DisallowUnknownFields()
 if err=dec.Decode(&req);err!=nil{fail(w,400,"invalid JSON");return}
 var extra any;if dec.Decode(&extra)!=io.EOF{fail(w,400,"invalid JSON");return}
 if !validURL(req.URL){fail(w,400,"invalid URL");return}
 if req.Alias!=nil && (!aliasPattern.MatchString(*req.Alias)||*req.Alias=="api"){fail(w,400,"invalid alias");return}
 s.store.mu.Lock();defer s.store.mu.Unlock()
 code:=""
 if req.Alias!=nil {code=*req.Alias;if _,ok:=s.store.links[code];ok{fail(w,409,"alias already taken");return}} else {
  for {code,err=randomCode();if err!=nil{fail(w,500,"could not generate code");return};if _,ok:=s.store.links[code];!ok{break}}
 }
 l:=Link{Code:code,URL:req.URL,CreatedAt:time.Now().UTC()}
 next:=s.store.copy();next[code]=l
 if err=s.store.save(next);err!=nil{log.Printf("save: %v",err);fail(w,500,"storage failure");return}
 jsonResponse(w,201,l)
}
func (s *Server) follow(w http.ResponseWriter,r *http.Request,code string) {
 s.store.mu.Lock();defer s.store.mu.Unlock()
 l,ok:=s.store.links[code];if !ok{fail(w,404,"not found");return}
 l.Visits++;next:=s.store.copy();next[code]=l
 if err:=s.store.save(next);err!=nil{log.Printf("save: %v",err);fail(w,500,"storage failure");return}
 w.Header().Set("Location",l.URL);w.WriteHeader(http.StatusFound)
}
func (s *Server) remove(w http.ResponseWriter,code string) {
 s.store.mu.Lock();defer s.store.mu.Unlock()
 if _,ok:=s.store.links[code];!ok{fail(w,404,"not found");return}
 next:=s.store.copy();delete(next,code)
 if err:=s.store.save(next);err!=nil{log.Printf("save: %v",err);fail(w,500,"storage failure");return}
 w.WriteHeader(204)
}
func main() {
 addr:=flag.String("addr",":8080","listen address");data:=flag.String("data","data.json","storage file");flag.Parse()
 token:=os.Getenv("ADMIN_TOKEN");if token==""{log.Fatal("ADMIN_TOKEN must be set and non-empty")}
 store,err:=openStore(*data);if err!=nil{log.Fatal(err)}
 server:=&http.Server{Addr:*addr,Handler:&Server{store:store,token:token},ReadHeaderTimeout:10*time.Second}
 log.Fatal(server.ListenAndServe())
}
