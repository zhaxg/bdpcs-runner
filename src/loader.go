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
	toolVersion = "2.1.0"
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

func rulesHash(css, js []byte) string {
	h := sha256.New()
	h.Write(css)
	h.Write([]byte{0})
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
    var css=__CSS__;
    var js=__JS__;
    var src='embedded';
    try{var c=fs.readFileSync(path.join(dir,'adblock.css'),'utf8');if(c){css=c;src='override'}}catch(e){}
    try{var j=fs.readFileSync(path.join(dir,'adblock.js'),'utf8');if(j){js=j;src='override'}}catch(e){}
    var seen={};
    function key(w){try{return w.id+'|'+w.getURL()}catch(e){return String(w.id)}}
    function apply(w){
      try{if(!w||w.isDestroyed())return}catch(e){return}
      var k=key(w);if(seen[k])return;seen[k]=1;
      try{var p=w.insertCSS(css);if(p&&p.catch)p.catch(function(){})}catch(e){log('insertCSS '+e.message)}
      try{var q=w.executeJavaScript(js);if(q&&q.catch)q.catch(function(){})}catch(e){log('executeJavaScript '+e.message)}
    }
    app.on('web-contents-created',function(e,w){
      try{
        w.on('dom-ready',function(){apply(w)});
        w.on('did-finish-load',function(){delete seen[key(w)];apply(w)});
      }catch(err){log('hook '+err.message)}
    });
    log('loader ready v__VER__ '+src+' css='+css.length+'B js='+js.length+'B');
  }catch(e){
    try{require('fs').appendFileSync(require('path').join(process.env.LOCALAPPDATA||process.env.TEMP||'.','BaiduNetdiskClean','adblock.log'),'FATAL '+e.stack+'\n')}catch(_){}
  }
})();`

func buildLoader(css, js []byte) []byte {
	cssLit, _ := json.Marshal(string(css))
	jsLit, _ := json.Marshal(string(js))
	s := loaderTpl
	s = strings.ReplaceAll(s, "__CSS__", string(cssLit))
	s = strings.ReplaceAll(s, "__JS__", string(jsLit))
	s = strings.ReplaceAll(s, "__VER__", toolVersion)
	return []byte(s)
}

func describeRules(css, js []byte) string {
	return fmt.Sprintf("css=%dB js=%dB rules=%s", len(css), len(js), rulesHash(css, js)[:12])
}