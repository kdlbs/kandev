import { describe, expect, it } from 'vitest';
import { detectPreviewUrl, detectPreviewUrlFromOutput } from './preview-url-detector';

describe('detectPreviewUrl', () => {
  describe('full URL patterns', () => {
    it('detects localhost URL with port', () => {
      const result = detectPreviewUrl('Server running at http://localhost:3000');
      expect(result).toEqual({
        url: 'http://localhost:3000/',
        port: 3000,
        scheme: 'http',
      });
    });

    it('detects 127.0.0.1 URL with port', () => {
      const result = detectPreviewUrl('Listening on http://127.0.0.1:8080');
      expect(result).toEqual({
        url: 'http://127.0.0.1:8080/',
        port: 8080,
        scheme: 'http',
      });
    });

    it('detects 0.0.0.0 URL with port', () => {
      const result = detectPreviewUrl('Server at http://0.0.0.0:4000');
      expect(result).toEqual({
        url: 'http://0.0.0.0:4000/',
        port: 4000,
        scheme: 'http',
      });
    });

    it('detects HTTPS URLs', () => {
      const result = detectPreviewUrl('Ready on https://localhost:3000');
      expect(result).toEqual({
        url: 'https://localhost:3000/',
        port: 3000,
        scheme: 'https',
      });
    });

    it('detects URLs with paths', () => {
      const result = detectPreviewUrl('Visit http://localhost:3000/admin');
      expect(result).toEqual({
        url: 'http://localhost:3000/admin',
        port: 3000,
        scheme: 'http',
      });
    });

    it('rejects localhost URL without port', () => {
      const result = detectPreviewUrl('Server running at http://localhost');
      expect(result).toBeNull();
    });

    it('rejects 127.0.0.1 URL without port', () => {
      const result = detectPreviewUrl('Available at http://127.0.0.1');
      expect(result).toBeNull();
    });

    it('rejects 0.0.0.0 URL without port', () => {
      const result = detectPreviewUrl('Listening on http://0.0.0.0');
      expect(result).toBeNull();
    });
  });

  describe('host:port patterns', () => {
    it('detects localhost:port pattern', () => {
      const result = detectPreviewUrl('Server started on localhost:3000');
      expect(result).toEqual({
        url: 'http://localhost:3000',
        port: 3000,
        scheme: 'http',
      });
    });

    it('detects 127.0.0.1:port pattern', () => {
      const result = detectPreviewUrl('Bound to 127.0.0.1:8080');
      expect(result).toEqual({
        url: 'http://127.0.0.1:8080',
        port: 8080,
        scheme: 'http',
      });
    });

    it('detects 0.0.0.0:port pattern', () => {
      const result = detectPreviewUrl('Listening 0.0.0.0:4000');
      expect(result).toEqual({
        url: 'http://0.0.0.0:4000',
        port: 4000,
        scheme: 'http',
      });
    });

    it('infers https from context', () => {
      const result = detectPreviewUrl('HTTPS server on localhost:3000');
      expect(result).toEqual({
        url: 'https://localhost:3000',
        port: 3000,
        scheme: 'https',
      });
    });

    it('handles multi-digit ports', () => {
      const result = detectPreviewUrl('Running on localhost:12345');
      expect(result).toEqual({
        url: 'http://localhost:12345',
        port: 12345,
        scheme: 'http',
      });
    });
  });

  describe('edge cases', () => {
    it('returns null for empty string', () => {
      expect(detectPreviewUrl('')).toBeNull();
    });

    it('returns null for non-matching text', () => {
      expect(detectPreviewUrl('Server is starting...')).toBeNull();
    });

    it('returns null for invalid URLs', () => {
      expect(detectPreviewUrl('Invalid: http://[::1]:abc')).toBeNull();
    });

    it('handles URLs with special characters', () => {
      const result = detectPreviewUrl('Ready: http://localhost:3000?debug=true');
      expect(result?.url).toBe('http://localhost:3000/?debug=true');
    });

    it('handles multiple URLs in one line (returns first valid)', () => {
      const result = detectPreviewUrl(
        'Server at http://localhost:3000 and http://localhost:3001'
      );
      expect(result?.port).toBe(3000);
    });

    it('handles ANSI color codes', () => {
      const result = detectPreviewUrl('\x1b[32mRunning at http://localhost:3000\x1b[0m');
      expect(result?.port).toBe(3000);
    });
  });

  describe('real-world examples', () => {
    it('detects Next.js dev server', () => {
      const result = detectPreviewUrl('  ▲ Local:        http://localhost:3000');
      expect(result?.url).toBe('http://localhost:3000/');
    });

    it('detects Vite dev server', () => {
      const result = detectPreviewUrl('  ➜  Local:   http://localhost:5173/');
      expect(result?.url).toBe('http://localhost:5173/');
    });

    it('detects Create React App', () => {
      const result = detectPreviewUrl('On Your Network:  http://localhost:3000');
      expect(result?.url).toBe('http://localhost:3000/');
    });

    it('detects Rails server', () => {
      const result = detectPreviewUrl('* Listening on http://127.0.0.1:3000');
      expect(result?.url).toBe('http://127.0.0.1:3000/');
    });

    it('detects Django dev server', () => {
      const result = detectPreviewUrl('Starting development server at http://127.0.0.1:8000/');
      expect(result?.url).toBe('http://127.0.0.1:8000/');
    });

    it('detects Express server', () => {
      const result = detectPreviewUrl('Server listening on localhost:3000');
      expect(result?.url).toBe('http://localhost:3000');
    });

    it('detects Flask dev server', () => {
      const result = detectPreviewUrl(' * Running on http://127.0.0.1:5000');
      expect(result?.url).toBe('http://127.0.0.1:5000/');
    });
  });
});

describe('detectPreviewUrlFromOutput', () => {
  it('returns null for empty output', () => {
    expect(detectPreviewUrlFromOutput('')).toBeNull();
  });

  it('finds URL in multi-line output', () => {
    const output = `
Starting server...
Compiling...
Server running at http://localhost:3000
Ready!
    `;
    expect(detectPreviewUrlFromOutput(output)).toBe('http://localhost:3000/');
  });

  it('returns the last valid URL when multiple exist', () => {
    const output = `
Starting on http://localhost:3000
Error: Port in use
Starting on http://localhost:3001
Ready!
    `;
    expect(detectPreviewUrlFromOutput(output)).toBe('http://localhost:3001/');
  });

  it('skips invalid URLs and finds valid ones', () => {
    const output = `
Trying http://localhost
Failed
Trying http://localhost:3000
Success!
    `;
    expect(detectPreviewUrlFromOutput(output)).toBe('http://localhost:3000/');
  });

  it('handles output with no valid URLs', () => {
    const output = `
Server starting...
Initializing...
Done
    `;
    expect(detectPreviewUrlFromOutput(output)).toBeNull();
  });

  it('handles real Next.js output', () => {
    const output = `
  ▲ Next.js 14.0.0
  - Local:        http://localhost:3000
  - Environments: .env

 ✓ Ready in 1.5s
    `;
    expect(detectPreviewUrlFromOutput(output)).toBe('http://localhost:3000/');
  });

  it('handles real Vite output', () => {
    const output = `
  VITE v5.0.0  ready in 500 ms

  ➜  Local:   http://localhost:5173/
  ➜  Network: use --host to expose
    `;
    expect(detectPreviewUrlFromOutput(output)).toBe('http://localhost:5173/');
  });

  it('ignores URLs without ports mixed with valid ones', () => {
    const output = `
Checking http://localhost
Port available
Starting on localhost:3000
Ready!
    `;
    expect(detectPreviewUrlFromOutput(output)).toBe('http://localhost:3000');
  });
});
