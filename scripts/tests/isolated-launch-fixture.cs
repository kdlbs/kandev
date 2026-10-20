using System;
using System.Diagnostics;
using System.IO;
using System.Net;
using System.Net.Sockets;
using System.Text;
using System.Threading;

// Real Windows processes and sockets, without starting a developer database.
public static class IsolatedLaunchFixture
{
    public static int Main(string[] args)
    {
        string home = Environment.GetEnvironmentVariable("KANDEV_HOME_DIR");
        string executable = Process.GetCurrentProcess().MainModule.FileName;
        string name = Path.GetFileNameWithoutExtension(executable);
        if (name == "make")
        {
            Console.Error.WriteLine("fixture build diagnostic");
            return int.Parse(Environment.GetEnvironmentVariable("KANDEV_FIXTURE_BUILD_EXIT"));
        }
        if (name == "go") return 0;
        string role = name == "node" ? "web" : "backend";
        if (args.Length > 0 && args[0].StartsWith("--descendant"))
        {
            File.WriteAllText(Path.Combine(home, args[0].Substring(2) + ".pid"), Process.GetCurrentProcess().Id.ToString());
            if (args[0] == "--descendant") Spawn(executable, "--descendant-grandchild");
            Thread.Sleep(120000);
            return 0;
        }
        string modePath = Path.Combine(Path.GetDirectoryName(executable), role + ".mode");
        string mode = File.Exists(modePath) ? File.ReadAllText(modePath) : "healthy";
        File.WriteAllText(Path.Combine(home, role + ".pid"), Process.GetCurrentProcess().Id.ToString());
        File.WriteAllText(Path.Combine(home, role + ".args"), string.Join("\n", args));
        File.WriteAllText(Path.Combine(home, role + ".environment"),
            Environment.GetEnvironmentVariable("KANDEV_DATABASE_DRIVER") + "|" +
            Environment.GetEnvironmentVariable("DOCKER_HOST") + "|" +
            Environment.GetEnvironmentVariable("VITE_SECRET_FOR_TEST"));
        if (mode.Contains("tree")) Spawn(executable, "--descendant");
        if (mode.Contains("delay"))
        {
            string release = Path.Combine(home, "release");
            var until = DateTime.UtcNow.AddSeconds(30);
            while (!File.Exists(release) && DateTime.UtcNow < until) Thread.Sleep(25);
        }
        if (mode.Contains("no-listener")) { Thread.Sleep(120000); return 0; }
        int port;
        string host = "127.0.0.1";
        if (role == "backend")
        {
            port = int.Parse(Environment.GetEnvironmentVariable("KANDEV_SERVER_PORT"));
            host = Environment.GetEnvironmentVariable("KANDEV_SERVER_HOST") ?? host;
        }
        else
        {
            int index = Array.IndexOf(args, "--port");
            port = int.Parse(args[index + 1]);
            host = args[Array.IndexOf(args, "--host") + 1];
        }
        IPAddress address;
        if (!IPAddress.TryParse(host.Trim('[', ']'), out address)) address = Dns.GetHostAddresses(host)[0];
        var listener = new TcpListener(address, port);
        listener.Start();
        Console.Error.WriteLine(role + " fixture diagnostic");
        while (true)
        {
            using (var client = listener.AcceptTcpClient())
            using (var stream = client.GetStream())
            {
                stream.ReadTimeout = 2000;
                var request = new byte[4096];
                try { stream.Read(request, 0, request.Length); } catch (IOException) { continue; }
                byte[] response = Encoding.ASCII.GetBytes("HTTP/1.1 " +
                    (mode.Contains("unhealthy") ? "503 Unavailable" : "200 OK") +
                    "\r\nContent-Length: 2\r\nConnection: close\r\n\r\nOK");
                stream.Write(response, 0, response.Length);
            }
        }
    }

    private static void Spawn(string executable, string arguments)
    {
        Process.Start(new ProcessStartInfo(executable, arguments) { UseShellExecute = false, CreateNoWindow = true });
    }
}
