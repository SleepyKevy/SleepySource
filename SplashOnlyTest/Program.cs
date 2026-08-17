using System.Diagnostics;
using System.Drawing.Drawing2D;
using System.Reflection;
using System.Runtime.InteropServices;

namespace SleepySource.SplashTest;

internal static class Program
{
    [STAThread]
    private static void Main()
    {
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
        Application.EnableVisualStyles();
        Application.SetCompatibleTextRenderingDefault(false);
        Application.Run(new SplashTestForm());
    }
}

internal sealed class SplashTestForm : Form
{
    private const int SourceWidth = 1670;
    private const int SourceHeight = 941;
    private static readonly RectangleF ButtonSourceRect = new(571, 774, 508, 77);
    private static readonly RectangleF TitleSourceRect = new(470, 560, 745, 95);

    private readonly Bitmap background;
    private readonly System.Windows.Forms.Timer animationTimer = new() { Interval = 16 };
    private readonly Stopwatch clock = Stopwatch.StartNew();
    private readonly List<Particle> particles = new();
    private readonly Random random = new(8127);
    private bool hoverButton;
    private bool closing;
    private bool fadeIn = true;

    [DllImport("user32.dll")]
    private static extern bool ReleaseCapture();

    [DllImport("user32.dll")]
    private static extern IntPtr SendMessage(IntPtr hWnd, int msg, IntPtr wParam, IntPtr lParam);

    public SplashTestForm()
    {
        Text = "SleepySource Splash Test";
        FormBorderStyle = FormBorderStyle.None;
        StartPosition = FormStartPosition.CenterScreen;
        BackColor = Color.Black;
        KeyPreview = true;
        DoubleBuffered = true;
        Opacity = 0;
        ShowInTaskbar = true;

        var area = Screen.PrimaryScreen?.WorkingArea ?? new Rectangle(0, 0, 1920, 1080);
        var maxWidth = Math.Min(1450, (int)(area.Width * 0.90));
        var maxHeight = (int)(area.Height * 0.90);
        var width = maxWidth;
        var height = (int)Math.Round(width * (SourceHeight / (double)SourceWidth));
        if (height > maxHeight)
        {
            height = maxHeight;
            width = (int)Math.Round(height * (SourceWidth / (double)SourceHeight));
        }
        ClientSize = new Size(Math.Max(900, width), Math.Max(507, height));

        background = LoadEmbeddedSplash();
        BuildParticles();

        animationTimer.Tick += (_, _) => TickAnimation();
        animationTimer.Start();

        MouseMove += OnMouseMove;
        MouseDown += OnMouseDown;
        MouseLeave += (_, _) => { hoverButton = false; Cursor = Cursors.Default; };
        KeyDown += OnKeyDown;
    }

    private static Bitmap LoadEmbeddedSplash()
    {
        using var stream = Assembly.GetExecutingAssembly().GetManifestResourceStream("SleepySource.SplashTest.splash.jpg")
            ?? throw new InvalidOperationException("Embedded splash artwork is missing.");
        using var image = Image.FromStream(stream);
        return new Bitmap(image);
    }

    private void BuildParticles()
    {
        particles.Clear();
        for (var i = 0; i < 25; i++)
        {
            particles.Add(new Particle
            {
                X = (float)(random.NextDouble() * SourceWidth),
                Y = (float)(random.NextDouble() * SourceHeight),
                DX = (float)(-0.11 + random.NextDouble() * 0.22),
                DY = (float)(-0.10 + random.NextDouble() * 0.15),
                Size = (float)(0.8 + random.NextDouble() * 2.2),
                Phase = (float)(random.NextDouble() * Math.PI * 2),
                Alpha = 18 + random.Next(42)
            });
        }
    }

    private void TickAnimation()
    {
        if (fadeIn)
        {
            Opacity = Math.Min(1, Opacity + 0.075);
            if (Opacity >= 0.999) fadeIn = false;
        }

        if (closing)
        {
            Opacity = Math.Max(0, Opacity - 0.085);
            if (Opacity <= 0.015)
            {
                animationTimer.Stop();
                Close();
                return;
            }
        }

        foreach (var p in particles)
        {
            p.X += p.DX;
            p.Y += p.DY;
            if (p.X < -15) p.X = SourceWidth + 15;
            if (p.X > SourceWidth + 15) p.X = -15;
            if (p.Y < -15) p.Y = SourceHeight + 15;
            if (p.Y > SourceHeight + 15) p.Y = -15;
        }

        Invalidate();
    }

    protected override void OnPaint(PaintEventArgs e)
    {
        base.OnPaint(e);
        var g = e.Graphics;
        g.SmoothingMode = SmoothingMode.AntiAlias;
        g.PixelOffsetMode = PixelOffsetMode.HighQuality;
        g.CompositingQuality = CompositingQuality.HighQuality;
        g.InterpolationMode = InterpolationMode.HighQualityBicubic;

        var imageRect = GetImageRect();
        g.Clear(Color.Black);
        g.DrawImage(background, imageRect);
        g.SetClip(imageRect);

        var t = clock.Elapsed.TotalSeconds;
        DrawAmbientFog(g, imageRect, t);
        DrawParticles(g, imageRect, t);
        DrawHaloMotion(g, imageRect, t);
        DrawLogoBreath(g, imageRect, t);
        DrawHorizonPulse(g, imageRect, t);
        DrawTitleShimmer(g, imageRect, t);
        DrawButtonGlow(g, imageRect, t);
        DrawReadyPulse(g, imageRect, t);

        g.ResetClip();
    }

    private RectangleF GetImageRect()
    {
        var client = ClientRectangle;
        var scale = Math.Min(client.Width / (float)SourceWidth, client.Height / (float)SourceHeight);
        var w = SourceWidth * scale;
        var h = SourceHeight * scale;
        return new RectangleF((client.Width - w) / 2f, (client.Height - h) / 2f, w, h);
    }

    private static PointF Map(RectangleF imageRect, float x, float y)
    {
        return new PointF(
            imageRect.Left + x / SourceWidth * imageRect.Width,
            imageRect.Top + y / SourceHeight * imageRect.Height);
    }

    private static RectangleF Map(RectangleF imageRect, RectangleF sourceRect)
    {
        var p = Map(imageRect, sourceRect.X, sourceRect.Y);
        return new RectangleF(
            p.X,
            p.Y,
            sourceRect.Width / SourceWidth * imageRect.Width,
            sourceRect.Height / SourceHeight * imageRect.Height);
    }

    private void DrawAmbientFog(Graphics g, RectangleF r, double t)
    {
        var drift = (float)Math.Sin(t * 0.20) * 22f;
        var breathe = 0.5f + 0.5f * (float)Math.Sin(t * 0.42);
        DrawGlow(g, Map(r, 270 + drift, 565), 255, 105, Color.FromArgb(0, 125, 255), 8 + (int)(breathe * 5));
        DrawGlow(g, Map(r, 1395 - drift * .7f, 555), 280, 110, Color.FromArgb(0, 155, 255), 7 + (int)(breathe * 5));
        DrawGlow(g, Map(r, 835, 575), 360, 82, Color.FromArgb(0, 150, 255), 10 + (int)(breathe * 7));
    }

    private void DrawParticles(Graphics g, RectangleF r, double t)
    {
        foreach (var p in particles)
        {
            var pulse = 0.45 + 0.55 * Math.Sin(t * 0.7 + p.Phase);
            var alpha = Math.Clamp((int)(p.Alpha * pulse), 4, 70);
            var pt = Map(r, p.X, p.Y);
            var size = p.Size / SourceWidth * r.Width;
            using var glow = new SolidBrush(Color.FromArgb(alpha / 3, 0, 150, 255));
            using var dot = new SolidBrush(Color.FromArgb(alpha, 75, 200, 255));
            g.FillEllipse(glow, pt.X - size * 2.2f, pt.Y - size * 2.2f, size * 4.4f, size * 4.4f);
            g.FillEllipse(dot, pt.X - size / 2f, pt.Y - size / 2f, size, size);
        }
    }

    private void DrawHaloMotion(Graphics g, RectangleF r, double t)
    {
        var center = Map(r, 835, 337);
        var scale = r.Width / SourceWidth;
        var radius = 326f * scale;
        var rect = new RectangleF(center.X - radius, center.Y - radius, radius * 2, radius * 2);
        using var pen1 = new Pen(Color.FromArgb(34, 0, 170, 255), Math.Max(1.2f, 1.7f * scale));
        using var pen2 = new Pen(Color.FromArgb(20, 80, 120, 255), Math.Max(1f, 1.2f * scale));
        var a = (float)((t * 7.2) % 360);
        g.DrawArc(pen1, rect, a, 76);
        g.DrawArc(pen1, rect, a + 132, 48);
        var inner = RectangleF.Inflate(rect, -28 * scale, -28 * scale);
        g.DrawArc(pen2, inner, 300 - a * .55f, 92);
        g.DrawArc(pen2, inner, 90 - a * .55f, 42);
    }

    private void DrawLogoBreath(Graphics g, RectangleF r, double t)
    {
        var pulse = 0.5 + 0.5 * Math.Sin(t * (Math.PI * 2 / 4.6));
        var center = Map(r, 835, 335);
        DrawGlow(g, center, 320, 275, Color.FromArgb(0, 174, 255), 9 + (int)(pulse * 10));
    }

    private void DrawHorizonPulse(Graphics g, RectangleF r, double t)
    {
        var pulse = (float)(0.5 + 0.5 * Math.Sin(t * (Math.PI * 2 / 6.2)));
        var p1 = Map(r, 420, 574);
        var p2 = Map(r, 1250, 574);
        var h = Math.Max(2f, 7f / SourceHeight * r.Height);
        var band = new RectangleF(p1.X, p1.Y - h / 2, p2.X - p1.X, h);
        using var brush = new LinearGradientBrush(band, Color.Transparent, Color.Transparent, 0f);
        brush.InterpolationColors = new ColorBlend
        {
            Positions = new[] { 0f, .28f, .5f, .72f, 1f },
            Colors = new[]
            {
                Color.Transparent,
                Color.FromArgb((int)(10 + pulse * 13), 0, 115, 255),
                Color.FromArgb((int)(28 + pulse * 34), 55, 210, 255),
                Color.FromArgb((int)(10 + pulse * 13), 0, 115, 255),
                Color.Transparent
            }
        };
        g.FillRectangle(brush, band);
    }

    private void DrawTitleShimmer(Graphics g, RectangleF r, double t)
    {
        const double period = 11.5;
        const double duration = 1.25;
        var phase = t % period;
        if (phase > duration) return;

        var title = Map(r, TitleSourceRect);
        var progress = (float)(phase / duration);
        var sweepWidth = Math.Max(36f, title.Width * .075f);
        var x = title.Left - sweepWidth + progress * (title.Width + sweepWidth * 2);
        var sweep = new RectangleF(x, title.Top - 4, sweepWidth, title.Height + 8);

        var state = g.Save();
        g.SetClip(title);
        using var brush = new LinearGradientBrush(sweep, Color.Transparent, Color.Transparent, 0f);
        brush.InterpolationColors = new ColorBlend
        {
            Positions = new[] { 0f, .45f, .52f, .60f, 1f },
            Colors = new[]
            {
                Color.Transparent,
                Color.FromArgb(4, 140, 225, 255),
                Color.FromArgb(25, 230, 250, 255),
                Color.FromArgb(4, 140, 225, 255),
                Color.Transparent
            }
        };
        g.FillRectangle(brush, sweep);
        g.Restore(state);
    }

    private void DrawButtonGlow(Graphics g, RectangleF r, double t)
    {
        var rect = Map(r, ButtonSourceRect);
        var idle = 0.5 + 0.5 * Math.Sin(t * (Math.PI * 2 / 3.8));
        var boost = hoverButton ? 1.0 : 0.0;
        var alpha = (int)(65 + idle * 50 + boost * 75);
        var scale = r.Width / SourceWidth;

        for (var i = 4; i >= 1; i--)
        {
            var expanded = RectangleF.Inflate(rect, i * 2.7f * scale, i * 2.0f * scale);
            using var pen = new Pen(Color.FromArgb(Math.Max(4, alpha / (i * 5)), 0, 155, 255), Math.Max(1f, i * 1.2f * scale));
            g.DrawPath(pen, RoundedRect(expanded, 16f * scale));
        }

        using var border = new Pen(Color.FromArgb(Math.Min(220, alpha), 55, 205, 255), Math.Max(1f, 1.5f * scale));
        g.DrawPath(border, RoundedRect(rect, 16f * scale));
    }

    private void DrawReadyPulse(Graphics g, RectangleF r, double t)
    {
        var center = Map(r, 1523, 43);
        var scale = r.Width / SourceWidth;
        var pulse = (float)(0.5 + 0.5 * Math.Sin(t * (Math.PI * 2 / 2.2)));
        var radius = (8 + 4 * pulse) * scale;
        using var glow = new SolidBrush(Color.FromArgb((int)(20 + pulse * 42), 28, 255, 165));
        g.FillEllipse(glow, center.X - radius, center.Y - radius, radius * 2, radius * 2);
    }

    private static void DrawGlow(Graphics g, PointF center, float sourceRadiusX, float sourceRadiusY, Color color, int strength)
    {
        var scaleX = g.VisibleClipBounds.Width / SourceWidth;
        var scaleY = g.VisibleClipBounds.Height / SourceHeight;
        var rx = sourceRadiusX * scaleX;
        var ry = sourceRadiusY * scaleY;
        for (var i = 9; i >= 1; i--)
        {
            var f = i / 9f;
            var alpha = Math.Clamp((int)(strength * (1f - f) * .65f), 1, 32);
            using var brush = new SolidBrush(Color.FromArgb(alpha, color.R, color.G, color.B));
            g.FillEllipse(brush, center.X - rx * f, center.Y - ry * f, rx * f * 2, ry * f * 2);
        }
    }

    private static GraphicsPath RoundedRect(RectangleF rect, float radius)
    {
        var d = Math.Min(Math.Min(radius * 2, rect.Width), rect.Height);
        var path = new GraphicsPath();
        path.AddArc(rect.Left, rect.Top, d, d, 180, 90);
        path.AddArc(rect.Right - d, rect.Top, d, d, 270, 90);
        path.AddArc(rect.Right - d, rect.Bottom - d, d, d, 0, 90);
        path.AddArc(rect.Left, rect.Bottom - d, d, d, 90, 90);
        path.CloseFigure();
        return path;
    }

    private void OnMouseMove(object? sender, MouseEventArgs e)
    {
        var hit = Map(GetImageRect(), ButtonSourceRect).Contains(e.Location);
        if (hit == hoverButton) return;
        hoverButton = hit;
        Cursor = hit ? Cursors.Hand : Cursors.Default;
        Invalidate();
    }

    private void OnMouseDown(object? sender, MouseEventArgs e)
    {
        if (e.Button != MouseButtons.Left) return;
        if (Map(GetImageRect(), ButtonSourceRect).Contains(e.Location))
        {
            BeginClose();
            return;
        }

        ReleaseCapture();
        _ = SendMessage(Handle, 0xA1, (IntPtr)0x2, IntPtr.Zero);
    }

    private void OnKeyDown(object? sender, KeyEventArgs e)
    {
        if (e.KeyCode == Keys.Enter)
        {
            e.Handled = true;
            BeginClose();
        }
        else if (e.KeyCode == Keys.Escape)
        {
            e.Handled = true;
            BeginClose();
        }
    }

    private void BeginClose()
    {
        if (closing) return;
        closing = true;
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            animationTimer.Dispose();
            background.Dispose();
        }
        base.Dispose(disposing);
    }

    private sealed class Particle
    {
        public float X;
        public float Y;
        public float DX;
        public float DY;
        public float Size;
        public float Phase;
        public int Alpha;
    }
}
