"""Assert physical A4 page count, footers, totals and text bounds using Poppler."""
import subprocess,xml.etree.ElementTree as E,json,sys
from pathlib import Path
out=Path(sys.argv[1]); results=[]
for name in ['delivery-draft-visible.pdf','delivery-draft-hidden.pdf','delivery-posted-snapshot.pdf','statement-43-entries.pdf']:
 xml=subprocess.check_output(['pdftotext','-bbox',str(out/name),'-'],text=True)
 root=E.fromstring(xml);ns={'x':'http://www.w3.org/1999/xhtml'};pages=root.findall('.//x:page',ns)
 texts=[''.join(w.text or '' for w in pg.findall('.//x:word',ns)) for pg in pages];expect=4 if name.startswith('statement') else 3
 assert len(pages)==expect,(name,len(pages),expect)
 for i,pg in enumerate(pages):
  width=float(pg.attrib['width']);height=float(pg.attrib['height']);assert abs(width-595)<2 and abs(height-842)<2
  for w in pg.findall('.//x:word',ns):
   assert float(w.attrib['xMin'])>=0 and float(w.attrib['yMin'])>=0
   assert float(w.attrib['xMax'])<=width+1 and float(w.attrib['yMax'])<=height+1,(name,i,w.text,w.attrib)
  assert f'第{i+1}/{expect}页' in texts[i],(name,i,texts[i][-100:])
 if name.startswith('statement'):
  assert '期末余额：370.00' in texts[-1],texts[-1][-500:];assert '期间净变动：370.00' in texts[-1];assert '43' in texts[-1]
 elif 'hidden' in name: assert '合计金额' not in ''.join(texts)
 else: assert '合计金额：¥820.00' in texts[-1];assert '收货人（签字）' in texts[-1]
 if 'draft' in name: assert all('账过未' in t or '未过账' in t for t in texts)
 elif not name.startswith('statement'): assert all('未过账' not in t and '账过未' not in t for t in texts)
 results.append({'file':name,'pages':len(pages),'a4':True,'allTextWithinPage':True,'lastPageTotalsChecked':True,'footerPagesChecked':True,'lastPageText':texts[-1]})
(out/'pdf-result.json').write_text(json.dumps(results,ensure_ascii=False,indent=2))
print(json.dumps([{k:v for k,v in x.items() if k!='lastPageText'} for x in results],ensure_ascii=False,indent=2))
for name,last in [('delivery-posted-snapshot',3),('statement-43-entries',4)]:subprocess.run(['pdftoppm','-f',str(last),'-l',str(last),'-scale-to','1400','-png','-singlefile',str(out/f'{name}.pdf'),str(out/f'{name}-last-page')],check=True)
