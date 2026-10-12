async page => {
 const baseURL = new URL(page.url()).origin;
 if (!/^http:\/\/(localhost|127\.0\.0\.1):/.test(baseURL) || baseURL.endsWith(':38429')) throw new Error('Use an isolated instance');
 const paths=['/?workflowId=journey-workflow-0','/tasks/journey-task-0000','/tasks/journey-task-0001','/tasks/journey-task-0001?sessionId=journey-task-0001-session-3'];
 const results=[];
 for(const path of paths){
  await page.goto(baseURL+path,{waitUntil:'domcontentloaded'});
  await page.getByText(/Journey task \d+/).first().waitFor({timeout:60000});
  await page.waitForTimeout(4000);
  const r=await page.evaluate(()=>({url:location.href,probe:window.__journeyProbe,resources:performance.getEntriesByType('resource').filter(r=>/\/api\//.test(r.name)).map(r=>({name:r.name,bytes:r.encodedBodySize,ms:r.duration})),visibleText:document.body.innerText.slice(0,1800)}));
  results.push(r);
 }
 return results;
}
