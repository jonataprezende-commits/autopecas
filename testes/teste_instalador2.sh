#!/bin/bash
set -u
export WINEPREFIX=/home/user/.cache/wineprefix
export WINEDEBUG=-all
DISP=:78
Xvfb $DISP -screen 0 1024x700x24 >/dev/null 2>&1 &
XVFB_PID=$!
sleep 2
export DISPLAY=$DISP
cd /home/user/dist
/usr/lib/wine/wine64 ./AutoPecas-Setup-1.0.0.exe >/home/user/qa/wine-setup.log 2>&1 &
sleep 15
echo "=== janelas:"
xdotool search --name "." getwindowname %@ 2>/dev/null | head -20
echo "=== processo:"; ps aux | grep "[A]utoPecas-Setup" | wc -l
echo "=== arvore de pastas do usuario wine:"
find /home/user/.cache/wineprefix/drive_c/users -maxdepth 6 -iname "*AutoPecas*" 2>/dev/null | head -10
xwd -root -display $DISP 2>/dev/null | convert xwd:- /home/user/qa/instalador-1.png
kill $XVFB_PID 2>/dev/null
pkill -f AutoPecas-Setup 2>/dev/null
