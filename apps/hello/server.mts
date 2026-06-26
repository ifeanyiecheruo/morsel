import { createServer } from 'node:http';

const port = Number(process.env.PORT) || 3000;

const server = createServer((_req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
  res.end(
    '<!doctype html>' +
    '<html><head><title>Hello</title></head>' +
    '<body><h1>Hello, World!</h1></body></html>',
  );
});

server.listen(port, () => {
  console.log(`listening on http://localhost:${port}`);
});
