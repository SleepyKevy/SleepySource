using System.Threading;

namespace SleepySource;

internal static class Program
{
    private const string MutexName = @"Local\SleepySource.CSharp.1.0.Beta";

    [STAThread]
    private static void Main()
    {
        using var mutex = new Mutex(initiallyOwned: true, MutexName, out var ownsMutex);
        if (!ownsMutex)
        {
            MessageBox.Show(
                "SleepySource 1.0 Beta is already running.",
                "SleepySource 1.0 Beta",
                MessageBoxButtons.OK,
                MessageBoxIcon.Information);
            return;
        }

        ApplicationConfiguration.Initialize();
        var mouseNavigationFilter = new MouseNavigationMessageFilter();
        Application.AddMessageFilter(mouseNavigationFilter);
        try
        {
            Application.Run(new MainForm());
        }
        finally
        {
            Application.RemoveMessageFilter(mouseNavigationFilter);
        }
    }

    private sealed class MouseNavigationMessageFilter : IMessageFilter
    {
        private const int WM_XBUTTONDOWN = 0x020B;
        private const int WM_XBUTTONUP = 0x020C;
        private const int WM_XBUTTONDBLCLK = 0x020D;
        private const int WM_APPCOMMAND = 0x0319;

        private const int APPCOMMAND_BROWSER_BACKWARD = 1;
        private const int APPCOMMAND_BROWSER_FORWARD = 2;
        private const int APPCOMMAND_MASK = 0x0FFF;

        public bool PreFilterMessage(ref Message m)
        {
            if (m.Msg == WM_XBUTTONDOWN ||
                m.Msg == WM_XBUTTONUP ||
                m.Msg == WM_XBUTTONDBLCLK)
            {
                return true;
            }

            if (m.Msg == WM_APPCOMMAND)
            {
                var command = (int)((m.LParam.ToInt64() >> 16) & APPCOMMAND_MASK);
                if (command == APPCOMMAND_BROWSER_BACKWARD ||
                    command == APPCOMMAND_BROWSER_FORWARD)
                {
                    return true;
                }
            }

            return false;
        }
    }
}
