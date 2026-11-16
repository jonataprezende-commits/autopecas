#!/bin/bash
set -u
export WINEPREFIX=/home/user/.cache/wineprefix
export WINEDEBUG=-all
DISP=:79
Xvfb $DISP -screen 0 1024x700x24 >/dev/null 2>&1 &
XVFB_PID=$!
sleep 2
export DISPLAY=$DISP
cd /home/user/dist
/usr/lib/wine/wine64 ./AutoPecas-Setup-1.0.0.exe >/home/user/qa/wine-setup.log 2>&1 &
sleep 14
xwd -root -display $DISP 2>/dev/null | convert xwd:- /home/user/qa/instalador-1.png
# clica em "Instalar agora" (botao em 170+300..460 x, 110+240..280 y)
xdotool mousemove 480 370 click 1
sleep 10
xwd -root -display $DISP 2>/dev/null | convert xwd:- /home/user/qa/instalador-2.png
# responde "Nao" na caixa de mensagem (Esc) duas vezes
xdotool key Escape; sleep 3
xwd -root -display $DISP 2>/dev/null | convert xwd:- /home/user/qa/instalador-3.png
xdotool key Escape; sleep 3
echo "=== instalado em:"
find /home/user/.cache/wineprefix/drive_c/users -ipath "*Programs/AutoPecas*" 2>/dev/null | head -10
echo "=== atalhos:"
find /home/user/.cache/wineprefix/drive_c/users -iname "*.lnk" 2>/dev/null | head -10
kill $XVFB_PID 2>/dev/null
pkill -f AutoPecas-Setup 2>/dev/null
