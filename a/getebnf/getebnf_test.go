package main

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestParseDecriptionHTML(t *testing.T) {
	const descriptionHTML = `
<!DOCTYPE html SYSTEM "about:legacy-compat">
<html xml:lang="en-us" lang="en-us"><head>
      <meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
      <link rel="stylesheet" href="Description%20of%20the%20illustration%20accessible_by_clause.eps_files/book.css">
      <meta name="viewport" content="width=device-width, initial-scale=1">
      <meta http-equiv="X-UA-Compatible" content="IE=edge">
      <title>Description of the illustration accessible_by_clause.eps</title>
      <meta name="generator" content="DITA Open Toolkit version 1.8.5 (Mode = doc)">
   
    <link rel="schema.dcterms" href="http://purl.org/dc/terms/">
    <meta name="dcterms.created" content="2021-08-20T22:56:18+00:00">
    <meta name="dcterms.title" content="Database PL/SQL Language Reference">
    <meta name="dcterms.category" content="database">
    <meta name="dcterms.isVersionOf" content="LNPLS">
    <meta name="dcterms.product" content="en/database/oracle/oracle-database/21">
    <meta name="dcterms.identifier" content="F31827-02">
    <meta name="dcterms.release" content="Release 21">
  <script id="ssot-metadata" type="application/json"> {"primary":{"category":{"short_name":"database","element_name":"Database","display_in_url":true},"suite":{"short_name":"oracle","element_name":"Oracle","display_in_url":true},"product_group":{"short_name":"not-applicable","element_name":"Not applicable","display_in_url":false},"product":{"short_name":"oracle-database","element_name":"Oracle Database","display_in_url":true},"release":{"short_name":"21","element_name":"Release 21","display_in_url":true}}} </script>
    <script>bazadebezolkohpepadr="77261192"</script><style>@media print {#ghostery-tracker-tally {display:none !important}}</style><script type="text/javascript" src="Description%20of%20the%20illustration%20accessible_by_clause.eps_files/49aea43" defer="defer"></script></head>
   <body>
      <article>
         <header>
            <h1>Description of the illustration accessible_by_clause.eps</h1>
         </header>
         <div><pre class="oac_no_warn" dir="ltr">ACCESSIBLE BY ( accessor [, accessor ]... )</pre></div>
      </article>
      
      <div class="footer copyrightlogo">
         <span><a href="https://docs.oracle.com/pls/topic/lookup?ctx=cpyr&amp;id=en">Copyright&nbsp;©&nbsp;1996, 2021, Oracle&nbsp;and/or&nbsp;its&nbsp;affiliates.&nbsp;</a></span></div>
   <noscript><img src="https://docs.oracle.com/akam/11/pixel_49aea43?a=dD1jYjVlNjViZWZkNGYzZmEwN2U2YWViMzcxMDA4OWQzYjY1NmM5MjgzJmpzPW9mZg==" style="visibility: hidden; position: absolute; left: -999px; top: -999px;" /></noscript>
</body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(descriptionHTML))
	if err != nil {
		t.Fatal(err)
	}
	var desc string
	doc.Find("body>article").Each(func(i int, s *goquery.Selection) {
		h1 := s.Find("header>h1").Text()
		pre := s.Find("div>pre").Text()
		t.Log("header>h1", h1, "div>pre", pre)
		if strings.HasPrefix(h1, "Description ") {
			desc = pre
		}
	})
	t.Log("DESCRIPTION ", desc)
	if desc == "" {
		t.Error("couldn't find description")
	}
}
