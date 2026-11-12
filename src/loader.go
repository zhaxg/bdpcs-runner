package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed assets/*
var assets embed.FS

const (
	toolVersion = "2.4.0"
)

func asset(name string) ([]byte, error) {
	return assets.ReadFile("assets/" + name)
}

func mustAsset(name string) []byte {
	b, err := asset(name)
	if err != nil {
		panic(err)
	}
	return b
}

func rulesHash(js []byte) string {
	h := sha256.New()
	h.Write(js)
	return hex.EncodeToString(h.Sum(nil))
}

var loaderTpl = `;(function(){
  try{
    var fs=require('fs'),path=require('path'),electron=require('electron');
    var app=electron.app;
    var dir=path.join(process.env.LOCALAPPDATA||process.env.TEMP||'.','BaiduNetdiskClean');
    try{fs.mkdirSync(dir,{recursive:true})}catch(e){}
    var logFile=path.join(dir,'adblock.log');
    function log(m){try{fs.appendFileSync(logFile,new Date().toISOString()+' '+m+'\n')}catch(e){}}
    var js=__JS__;
    var src='embedded';
    var ext='';
    try{ext=process.env.NDC_JS||''}catch(e){}
    if(ext){try{var j=fs.readFileSync(ext,'utf8');if(j){js=j;src=ext}}catch(e){}}
    if(src==='embedded'){
      try{var j2=fs.readFileSync(path.join(dir,'BaiduNetdiskRunner.js'),'utf8');if(j2){js=j2;src='runtime'}}catch(e){}
    }
    var seen={};
    function key(w){try{return w.id+'|'+w.getURL()}catch(e){return String(w.id)}}
    function apply(w){
      try{if(!w||w.isDestroyed())return}catch(e){return}
      var k=key(w);if(seen[k])return;seen[k]=1;
      try{var q=w.executeJavaScript(js);if(q&&q.catch)q.catch(function(err){log('rules '+err.message)})}catch(e){log('executeJavaScript '+e.message)}
    }
    function toggleDevTools(w,ev,key){
      try{
        if(w.isDevToolsOpened()){w.closeDevTools();log('devtools closed')}
        else{w.openDevTools({mode:'detach',activate:true});log('devtools opened ('+key+')')}
        ev.preventDefault();
      }catch(e){log('devtools '+e.message)}
    }
    app.on('web-contents-created',function(e,w){
      try{
        w.on('before-input-event',function(ev,input){
          try{
            if(!input||input.type!=='keyDown'||input.isAutoRepeat)return;
            var key=String(input.key||'').toUpperCase();
            if(key==='F12'||(input.control&&input.shift&&key==='I')){
              toggleDevTools(w,ev,key);
            }
          }catch(e){log('devtools '+e.message)}
        });
        w.on('dom-ready',function(){apply(w)});
        w.on('did-finish-load',function(){delete seen[key(w)];apply(w)});
      }catch(err){log('hook '+err.message)}
    });
    log('loader ready v__VER__ '+src+' js='+js.length+'B');
  }catch(e){
    try{require('fs').appendFileSync(require('path').join(process.env.LOCALAPPDATA||process.env.TEMP||'.','BaiduNetdiskClean','adblock.log'),'FATAL '+e.stack+'\n')}catch(_){}
  }
})();`

func buildLoader(js []byte) []byte {
	jsLit, _ := json.Marshal(string(js))
	s := loaderTpl
	s = strings.ReplaceAll(s, "__JS__", string(jsLit))
	s = strings.ReplaceAll(s, "__VER__", toolVersion)
	return []byte(s)
}

func describeRules(js []byte) string {
	return fmt.Sprintf("js=%dB rules=%s", len(js), rulesHash(js)[:12])
}
