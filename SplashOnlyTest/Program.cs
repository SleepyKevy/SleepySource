using System.Diagnostics;
using System.Drawing.Drawing2D;
using System.Drawing.Text;
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
    private const float DesignWidth = 1670f;
    private const float DesignHeight = 941f;
    private static readonly RectangleF ButtonDesign = new(570, 773, 510, 78);

    private readonly Bitmap logo;
    private readonly System.Windows.Forms.Timer timer = new() { Interval = 16 };
    private readonly Stopwatch clock = Stopwatch.StartNew();
    private readonly Random random = new(8021);
    private readonly List<Particle> particles = new();
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
        BackColor = Color.FromArgb(1, 4, 10);
        KeyPreview = true;
        DoubleBuffered = true;
        Opacity = 0;
        ShowInTaskbar = true;

        var area = Screen.PrimaryScreen?.WorkingArea ?? new Rectangle(0, 0, 1920, 1080);
        var width = Math.Min(1450, (int)(area.Width * .9));
        var height = (int)Math.Round(width * DesignHeight / DesignWidth);
        var maxHeight = (int)(area.Height * .9);
        if (height > maxHeight)
        {
            height = maxHeight;
            width = (int)Math.Round(height * DesignWidth / DesignHeight);
        }
        ClientSize = new Size(Math.Max(900, width), Math.Max(507, height));

        logo = LoadLogo();
        BuildParticles();

        timer.Tick += (_, _) => TickAnimation();
        timer.Start();
        MouseMove += OnMouseMove;
        MouseLeave += (_, _) => { hoverButton = false; Cursor = Cursors.Default; };
        MouseDown += OnMouseDown;
        KeyDown += OnKeyDown;
    }

    private static Bitmap LoadLogo()
    {
        using var stream = Assembly.GetExecutingAssembly().GetManifestResourceStream("SleepySource.SplashTest.default.png")
            ?? throw new InvalidOperationException("The SleepySource logo is missing from the test build.");
        using var image = Image.FromStream(stream);
        return new Bitmap(image);
    }

    private void BuildParticles()
    {
        for (var i = 0; i < 31; i++)
        {
            particles.Add(new Particle
            {
                X = (float)(random.NextDouble() * DesignWidth),
                Y = 45 + (float)(random.NextDouble() * 690),
                DX = (float)(-0.075 + random.NextDouble() * .15),
                DY = (float)(-0.06 + random.NextDouble() * .10),
                Size = .8f + (float)random.NextDouble() * 2.6f,
                Alpha = random.Next(25, 86),
                Phase = (float)(random.NextDouble() * Math.PI * 2)
            });
        }
    }

    private void TickAnimation()
    {
        if (fadeIn)
        {
            Opacity = Math.Min(1, Opacity + .07);
            if (Opacity >= .999) fadeIn = false;
        }
        if (closing)
        {
            Opacity = Math.Max(0, Opacity - .09);
            if (Opacity <= .015)
            {
                timer.Stop();
                Close();
                return;
            }
        }

        foreach (var p in particles)
        {
            p.X += p.DX;
            p.Y += p.DY;
            if (p.X < -10) p.X = DesignWidth + 10;
            if (p.X > DesignWidth + 10) p.X = -10;
            if (p.Y < 20) p.Y = 740;
            if (p.Y > 760) p.Y = 20;
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
        g.TextRenderingHint = TextRenderingHint.ClearTypeGridFit;

        var stage = GetStage();
        var state = g.Save();
        g.SetClip(stage);
        DrawBackground(g, stage);

        var t = clock.Elapsed.TotalSeconds;
        DrawAmbientFog(g, stage, t);
        DrawParticles(g, stage, t);
        DrawPerspectiveFloor(g, stage, t);
        DrawHalo(g, stage, t);
        DrawLogo(g, stage, t);
        DrawTitle(g, stage, t);
        DrawTagline(g, stage);
        DrawFeatures(g, stage);
        DrawButton(g, stage, t);
        DrawFooter(g, stage);
        DrawBuildLabel(g, stage);
        DrawReady(g, stage, t);
        g.Restore(state);
    }

    private RectangleF GetStage()
    {
        var c = ClientRectangle;
        var scale = Math.Min(c.Width / DesignWidth, c.Height / DesignHeight);
        var w = DesignWidth * scale;
        var h = DesignHeight * scale;
        return new RectangleF((c.Width - w) / 2f, (c.Height - h) / 2f, w, h);
    }

    private static PointF P(RectangleF s, float x, float y) =>
        new(s.Left + x / DesignWidth * s.Width, s.Top + y / DesignHeight * s.Height);

    private static RectangleF R(RectangleF s, RectangleF r)
    {
        var p = P(s, r.X, r.Y);
        return new RectangleF(p.X, p.Y, r.Width / DesignWidth * s.Width, r.Height / DesignHeight * s.Height);
    }

    private static float Scale(RectangleF s) => s.Width / DesignWidth;

    private static void DrawBackground(Graphics g, RectangleF s)
    {
        using var bg = new LinearGradientBrush(s, Color.FromArgb(1, 4, 12), Color.FromArgb(2, 9, 24), 90f);
        g.FillRectangle(bg, s);
        DrawGlow(g, P(s, 835, 285), 690 * Scale(s), 430 * Scale(s), Color.FromArgb(0, 75, 190), 58);
        DrawGlow(g, P(s, 835, 575), 610 * Scale(s), 125 * Scale(s), Color.FromArgb(0, 128, 255), 42);
    }

    private static void DrawAmbientFog(Graphics g, RectangleF s, double t)
    {
        var drift = (float)Math.Sin(t * .19) * 24;
        var pulse = .5f + .5f * (float)Math.Sin(t * .42);
        DrawGlow(g, P(s, 270 + drift, 545), 315 * Scale(s), 118 * Scale(s), Color.FromArgb(0, 115, 245), 18 + (int)(pulse * 10));
        DrawGlow(g, P(s, 1390 - drift * .7f, 545), 315 * Scale(s), 118 * Scale(s), Color.FromArgb(0, 150, 255), 16 + (int)(pulse * 9));
    }

    private void DrawParticles(Graphics g, RectangleF s, double t)
    {
        foreach (var p in particles)
        {
            var twinkle = .42 + .58 * Math.Sin(t * .75 + p.Phase);
            var alpha = Math.Clamp((int)(p.Alpha * twinkle), 7, 92);
            var pt = P(s, p.X, p.Y);
            var size = p.Size * Scale(s);
            using var glow = new SolidBrush(Color.FromArgb(alpha / 4, 0, 155, 255));
            using var dot = new SolidBrush(Color.FromArgb(alpha, 65, 190, 255));
            g.FillEllipse(glow, pt.X - size * 2.5f, pt.Y - size * 2.5f, size * 5, size * 5);
            g.FillEllipse(dot, pt.X - size / 2, pt.Y - size / 2, size, size);
        }
    }

    private static void DrawPerspectiveFloor(Graphics g, RectangleF s, double t)
    {
        var scale = Scale(s);
        var horizon = P(s, 835, 595);
        using var horizonPen = new Pen(Color.FromArgb(65, 0, 145, 255), Math.Max(1f, 1.3f * scale));
        g.DrawLine(horizonPen, P(s, 0, 595), P(s, 1670, 595));

        using var rayPen = new Pen(Color.FromArgb(25, 0, 110, 255), Math.Max(.7f, scale));
        for (var i = -8; i <= 8; i++)
        {
            var bottom = P(s, 835 + i * 110, 941);
            g.DrawLine(rayPen, horizon, bottom);
        }
        var offset = (float)(t * 10 % 50);
        for (var y = 645f + offset; y < 941; y += 55)
        {
            var fade = Math.Clamp((int)(18 + (y - 645) / 296 * 22), 12, 42);
            using var line = new Pen(Color.FromArgb(fade, 0, 100, 220), Math.Max(.6f, scale));
            g.DrawLine(line, P(s, 0, y), P(s, 1670, y));
        }
    }

    private static void DrawHalo(Graphics g, RectangleF s, double t)
    {
        var scale = Scale(s);
        var center = P(s, 835, 327);
        var radius = 290 * scale;
        var ring = new RectangleF(center.X - radius, center.Y - radius, radius * 2, radius * 2);
        var a = (float)(t * 6.5 % 360);
        using var p1 = new Pen(Color.FromArgb(38, 0, 176, 255), Math.Max(1f, 1.6f * scale));
        using var p2 = new Pen(Color.FromArgb(21, 50, 105, 255), Math.Max(.8f, 1.15f * scale));
        g.DrawArc(p1, ring, a, 72);
        g.DrawArc(p1, ring, a + 122, 51);
        g.DrawArc(p1, ring, a + 244, 38);
        var inner = RectangleF.Inflate(ring, -28 * scale, -28 * scale);
        g.DrawArc(p2, inner, 300 - a * .55f, 95);
        g.DrawArc(p2, inner, 90 - a * .55f, 48);
    }

    private void DrawLogo(Graphics g, RectangleF s, double t)
    {
        var scale = Scale(s);
        var pulse = .5f + .5f * (float)Math.Sin(t * Math.PI * 2 / 4.6);
        var center = P(s, 835, 323);
        var logoSize = 430 * scale;
        DrawGlow(g, center, (240 + pulse * 18) * scale, (230 + pulse * 18) * scale, Color.FromArgb(0, 185, 255), 37 + (int)(pulse * 18));
        var rect = new RectangleF(center.X - logoSize / 2, center.Y - logoSize / 2, logoSize, logoSize);
        g.DrawImage(logo, rect);
    }

    private static void DrawTitle(Graphics g, RectangleF s, double t)
    {
        var scale = Scale(s);
        using var font = new Font("Segoe UI", 64 * scale, FontStyle.Bold, GraphicsUnit.Pixel);
        const string text = "SleepySource";
        var size = g.MeasureString(text, font);
        var pos = P(s, 835, 565);
        var rect = new RectangleF(pos.X - size.Width / 2, pos.Y, size.Width, size.Height);
        using var fill = new LinearGradientBrush(rect, Color.FromArgb(250, 252, 255), Color.FromArgb(113, 191, 255), 90f);
        using var shadow = new SolidBrush(Color.FromArgb(30, 0, 150, 255));
        g.DrawString(text, font, shadow, rect.X + 2 * scale, rect.Y + 3 * scale);
        g.DrawString(text, font, fill, rect.Location);

        using var tmFont = new Font("Segoe UI", 18 * scale, FontStyle.Bold, GraphicsUnit.Pixel);
        using var tmBrush = new SolidBrush(Color.FromArgb(210, 213, 230, 255));
        g.DrawString("™", tmFont, tmBrush, rect.Right + 3 * scale, rect.Top + 2 * scale);

        var period = 11.5;
        var phase = t % period;
        if (phase < 1.25)
        {
            var progress = (float)(phase / 1.25);
            var sweepX = rect.Left - 45 * scale + progress * (rect.Width + 90 * scale);
            var sweep = new RectangleF(sweepX, rect.Top, 55 * scale, rect.Height);
            var saved = g.Save();
            g.SetClip(rect);
            using var sh = new LinearGradientBrush(sweep, Color.Transparent, Color.Transparent, 0f);
            sh.InterpolationColors = new ColorBlend
            {
                Positions = new[] { 0f, .45f, .52f, .60f, 1f },
                Colors = new[] { Color.Transparent, Color.FromArgb(8, 130, 220, 255), Color.FromArgb(38, 240, 252, 255), Color.FromArgb(8, 130, 220, 255), Color.Transparent }
            };
            g.FillRectangle(sh, sweep);
            g.Restore(saved);
        }
    }

    private static void DrawTagline(Graphics g, RectangleF s)
    {
        var scale = Scale(s);
        using var font = new Font("Segoe UI", 24 * scale, FontStyle.Regular, GraphicsUnit.Pixel);
        using var brush = new SolidBrush(Color.FromArgb(220, 91, 205, 255));
        DrawCentered(g, "Streaming Tools, All in One Place", font, brush, P(s, 835, 680));
        using var pen = new Pen(Color.FromArgb(85, 0, 145, 255), Math.Max(1f, scale));
        g.DrawLine(pen, P(s, 420, 693), P(s, 560, 693));
        g.DrawLine(pen, P(s, 1110, 693), P(s, 1250, 693));
    }

    private static void DrawFeatures(Graphics g, RectangleF s)
    {
        var scale = Scale(s);
        using var font = new Font("Segoe UI", 18 * scale, FontStyle.Regular, GraphicsUnit.Pixel);
        using var main = new SolidBrush(Color.FromArgb(205, 199, 213, 235));
        using var cyan = new SolidBrush(Color.FromArgb(235, 35, 190, 255));
        var labels = new[] { "Now Playing", "Chat Overlay", "Countdown Pro", "Kick Integration" };
        var widths = labels.Select(x => g.MeasureString(x, font).Width).ToArray();
        var dotWidth = g.MeasureString("  •  ", font).Width;
        var total = widths.Sum() + dotWidth * 3;
        var x = P(s, 835, 725).X - total / 2;
        var y = P(s, 835, 725).Y;
        for (var i = 0; i < labels.Length; i++)
        {
            g.DrawString(labels[i], font, main, x, y);
            x += widths[i];
            if (i < labels.Length - 1)
            {
                g.DrawString("  •  ", font, cyan, x, y);
                x += dotWidth;
            }
        }
    }

    private void DrawButton(Graphics g, RectangleF s, double t)
    {
        var scale = Scale(s);
        var rect = R(s, ButtonDesign);
        var pulse = .5f + .5f * (float)Math.Sin(t * Math.PI * 2 / 3.8);
        var boost = hoverButton ? 1f : 0f;
        var glowAlpha = (int)(35 + pulse * 30 + boost * 65);

        for (var i = 5; i >= 1; i--)
        {
            var grow = RectangleF.Inflate(rect, i * 3 * scale, i * 2.2f * scale);
            using var glowPen = new Pen(Color.FromArgb(Math.Max(3, glowAlpha / (i * 3)), 0, 145, 255), Math.Max(1f, i * scale));
            using var path = Round(grow, 16 * scale);
            g.DrawPath(glowPen, path);
        }

        using var pathMain = Round(rect, 16 * scale);
        using var bg = new LinearGradientBrush(rect,
            hoverButton ? Color.FromArgb(228, 8, 36, 72) : Color.FromArgb(225, 4, 24, 53),
            Color.FromArgb(242, 2, 11, 28), 90f);
        g.FillPath(bg, pathMain);
        using var border = new Pen(Color.FromArgb(hoverButton ? 235 : 190, 32, 177, 255), Math.Max(1f, 1.6f * scale));
        g.DrawPath(border, pathMain);

        using var font = new Font("Segoe UI", 25 * scale, FontStyle.Regular, GraphicsUnit.Pixel);
        using var text = new SolidBrush(Color.FromArgb(244, 232, 244, 255));
        DrawCentered(g, "Open SleepySource", font, text, new PointF(rect.Left + rect.Width / 2, rect.Top + 21 * scale));
    }

    private static void DrawFooter(Graphics g, RectangleF s)
    {
        var scale = Scale(s);
        using var font = new Font("Segoe UI", 18 * scale, FontStyle.Regular, GraphicsUnit.Pixel);
        using var brush = new SolidBrush(Color.FromArgb(180, 91, 149, 210));
        DrawCentered(g, "Made by SleepyKev  •  2026", font, brush, P(s, 835, 882));
    }

    private static void DrawBuildLabel(Graphics g, RectangleF s)
    {
        var scale = Scale(s);
        var rect = R(s, new RectangleF(26, 27, 170, 38));
        using var path = Round(rect, 6 * scale);
        using var bg = new SolidBrush(Color.FromArgb(80, 2, 13, 31));
        using var pen = new Pen(Color.FromArgb(110, 39, 154, 255), Math.Max(1f, scale));
        g.FillPath(bg, path);
        g.DrawPath(pen, path);
        using var font = new Font("Segoe UI", 16 * scale, FontStyle.Regular, GraphicsUnit.Pixel);
        using var brush = new SolidBrush(Color.FromArgb(205, 95, 180, 235));
        g.DrawString("Splash Test Build", font, brush, rect.Left + 10 * scale, rect.Top + 8 * scale);
    }

    private static void DrawReady(Graphics g, RectangleF s, double t)
    {
        var scale = Scale(s);
        var pulse = .5f + .5f * (float)Math.Sin(t * Math.PI * 2 / 2.2);
        var center = P(s, 1526, 43);
        var radius = (9 + pulse * 5) * scale;
        using var glow = new SolidBrush(Color.FromArgb((int)(18 + pulse * 52), 20, 255, 162));
        using var dot = new SolidBrush(Color.FromArgb(245, 28, 239, 145));
        g.FillEllipse(glow, center.X - radius, center.Y - radius, radius * 2, radius * 2);
        var d = 7 * scale;
        g.FillEllipse(dot, center.X - d / 2, center.Y - d / 2, d, d);
        using var font = new Font("Segoe UI", 17 * scale, FontStyle.Regular, GraphicsUnit.Pixel);
        using var brush = new SolidBrush(Color.FromArgb(245, 45, 245, 160));
        g.DrawString("Ready", font, brush, center.X + 14 * scale, center.Y - 11 * scale);
    }

    private static void DrawCentered(Graphics g, string value, Font font, Brush brush, PointF centerTop)
    {
        var size = g.MeasureString(value, font);
        g.DrawString(value, font, brush, centerTop.X - size.Width / 2, centerTop.Y);
    }

    private static GraphicsPath Round(RectangleF rect, float radius)
    {
        var d = Math.Min(radius * 2, Math.Min(rect.Width, rect.Height));
        var p = new GraphicsPath();
        p.AddArc(rect.Left, rect.Top, d, d, 180, 90);
        p.AddArc(rect.Right - d, rect.Top, d, d, 270, 90);
        p.AddArc(rect.Right - d, rect.Bottom - d, d, d, 0, 90);
        p.AddArc(rect.Left, rect.Bottom - d, d, d, 90, 90);
        p.CloseFigure();
        return p;
    }

    private static void DrawGlow(Graphics g, PointF center, float rx, float ry, Color color, int strength)
    {
        for (var i = 10; i >= 1; i--)
        {
            var f = i / 10f;
            var alpha = Math.Clamp((int)(strength * Math.Pow(1 - f, 1.45)), 1, 70);
            using var b = new SolidBrush(Color.FromArgb(alpha, color.R, color.G, color.B));
            g.FillEllipse(b, center.X - rx * f, center.Y - ry * f, rx * f * 2, ry * f * 2);
        }
    }

    private void OnMouseMove(object? sender, MouseEventArgs e)
    {
        var hit = R(GetStage(), ButtonDesign).Contains(e.Location);
        if (hit == hoverButton) return;
        hoverButton = hit;
        Cursor = hit ? Cursors.Hand : Cursors.Default;
        Invalidate();
    }

    private void OnMouseDown(object? sender, MouseEventArgs e)
    {
        if (e.Button != MouseButtons.Left) return;
        if (R(GetStage(), ButtonDesign).Contains(e.Location))
        {
            BeginClose();
            return;
        }
        ReleaseCapture();
        _ = SendMessage(Handle, 0xA1, (IntPtr)0x2, IntPtr.Zero);
    }

    private void OnKeyDown(object? sender, KeyEventArgs e)
    {
        if (e.KeyCode is Keys.Enter or Keys.Escape)
        {
            e.Handled = true;
            BeginClose();
        }
    }

    private void BeginClose()
    {
        if (!closing) closing = true;
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            timer.Dispose();
            logo.Dispose();
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
        public int Alpha;
        public float Phase;
    }
}
