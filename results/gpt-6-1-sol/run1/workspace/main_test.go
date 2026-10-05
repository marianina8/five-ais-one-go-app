package main

import (
 "encoding/json"
 "fmt"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "sync"
 "testing"
)
func setup(t *testing.T) *Server {t.Helper();s,err:=openStore(filepath.Join(t.TempDir(),"data.json"));if err!=nil{t.Fatal(err)};return &Server{store:s,token:"secret"}}
func request(s *Server,method,path,body,token string)*httptest.ResponseRecorder {r:=httptest.NewRequest(method,path,strings.NewReader(body));if token!=""{r.Header.Set("Authorization","Bearer "+token)};w:=httptest.NewRecorder();s.ServeHTTP(w,r);return w}
func TestLifecycle(t *testing.T) {
 s:=setup(t)
 w:=request(s,"POST","/api/links",`{"url":"https://example.com/page","alias":"my-page"}`,"");if w.Code!=201{t.Fatal(w.Code,w.Body.String())}
 var l Link;if err:=json.Unmarshal(w.Body.Bytes(),&l);err!=nil{t.Fatal(err)};if l.Code!="my-page"||l.Visits!=0||l.CreatedAt.IsZero(){t.Fatal(l)}
 if w=request(s,"POST","/api/links",`{"url":"https://example.com","alias":"my-page"}`,"");w.Code!=409{t.Fatal(w.Code)}
 if w=request(s,"GET","/my-page","","");w.Code!=302||w.Header().Get("Location")!="https://example.com/page"{t.Fatal(w)}
 loaded,err:=openStore(s.store.path);if err!=nil{t.Fatal(err)};if loaded.links["my-page"].Visits!=1{t.Fatal(loaded.links)}
 for _,token:=range []string{"","wrong"}{if w=request(s,"GET","/api/links","",token);w.Code!=401{t.Fatal(w.Code)}}
 w=request(s,"GET","/api/links","","secret");if w.Code!=200{t.Fatal(w.Code)}
 if w=request(s,"DELETE","/api/links/my-page","","secret");w.Code!=204||w.Body.Len()!=0{t.Fatal(w)}
 if w=request(s,"GET","/my-page","","");w.Code!=404{t.Fatal(w.Code)}
 loaded,err=openStore(s.store.path);if err!=nil||len(loaded.links)!=0{t.Fatal(loaded,err)}
}
func TestValidation(t *testing.T) {
 s:=setup(t)
 for _,body:=range []string{`{`, `null`, `{} `, `{"url":"/relative"}`, `{"url":"ftp://example.com"}`, `{"url":"https://"}`, `{"url":"https://example.com","alias":""}`, `{"url":"https://example.com","alias":"api"}`, `{"url":"https://example.com","alias":"a.b"}`, `{"url":"https://example.com","extra":1}`, `{"url":"https://example.com"} {}`,fmt.Sprintf(`{"url":%q}`,"https://example.com/"+strings.Repeat("x",2048))} {
  if w:=request(s,"POST","/api/links",body,"");w.Code!=400{t.Errorf("%s: %d",body,w.Code)}
 }
 if w:=request(s,"POST","/api/links",strings.Repeat(" ",1<<20)+"x","");w.Code!=413{t.Fatal(w.Code)}
 for _,p:=range []string{"/api/links","/api/links/abc","/abc"}{if w:=request(s,"PUT",p,"","");w.Code!=405{t.Fatal(p,w.Code)}}
 if w:=request(s,"GET","/no/such/path","","");w.Code!=404{t.Fatal(w.Code)}
}
func TestConcurrentPersistence(t *testing.T) {
 s:=setup(t)
 const n=30
 var wg sync.WaitGroup
 for i:=0;i<n;i++{wg.Add(1);go func(){defer wg.Done();w:=request(s,"POST","/api/links",`{"url":"https://example.com"}`,"");if w.Code!=201{t.Errorf("create %d",w.Code)}}()};wg.Wait()
 s.store.mu.Lock();links:=sortedLinks(s.store.links);s.store.mu.Unlock();if len(links)!=n{t.Fatal(len(links))}
 code:=links[0].Code;if len(code)!=7{t.Fatal(code)}
 for i:=0;i<n;i++{wg.Add(1);go func(){defer wg.Done();if w:=request(s,"GET","/"+code,"","");w.Code!=302{t.Errorf("follow %d",w.Code)}}()};wg.Wait()
 loaded,err:=openStore(s.store.path);if err!=nil{t.Fatal(err)};if len(loaded.links)!=n||loaded.links[code].Visits!=n{t.Fatal(loaded.links)}
 for i:=1;i<len(links);i++{if links[i].CreatedAt.Before(links[i-1].CreatedAt){t.Fatal("not sorted")}}
}
func TestStorageFailure(t *testing.T) {
 s:=setup(t);s.store.path=filepath.Join(t.TempDir(),"missing","data.json")
 if w:=request(s,"POST","/api/links",`{"url":"https://example.com"}`,"");w.Code!=500{t.Fatal(w.Code)}
 if len(s.store.links)!=0{t.Fatal("failed write changed memory")}
}
